package recheck

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setup(t *testing.T) (*Service, *gorm.DB, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/recheck.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "recheck-a", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "recheck-b", PasswordHash: "x", BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)
	return NewService(db), db, uid, other, pid
}

type docFixture struct {
	Doc     store.Document
	Source  store.ProjectSource
	Version store.SourceVersion
}

func seedDocument(t *testing.T, db *gorm.DB, projectID, ownerID store.UUID) docFixture {
	t.Helper()
	src := store.ProjectSource{ProjectID: projectID, Kind: "document", CurrentRevision: 1}
	require.NoError(t, db.Create(&src).Error)
	ver := store.SourceVersion{
		SourceID: src.ID, Revision: 1, S3Key: "k", ContentSHA256: "abc", SizeBytes: 1,
	}
	require.NoError(t, db.Create(&ver).Error)
	doc := store.Document{
		Name: "论文.pdf", Type: "application/pdf", Size: 1, Status: "uploaded",
		S3Key: "k", ProjectID: projectID, OwnerID: ownerID, SourceID: &src.ID,
	}
	require.NoError(t, db.Create(&doc).Error)
	return docFixture{Doc: doc, Source: src, Version: ver}
}

func seedArtifact(t *testing.T, db *gorm.DB, projectID, uid store.UUID, kind string, payload any,
	deps []store.ResearchArtifactDependency) (store.ResearchArtifact, store.ResearchArtifactRevision) {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	artifact := store.ResearchArtifact{
		ProjectID: projectID, Kind: kind, State: "active", CurrentRevision: 1, CreatedBy: uid,
	}
	require.NoError(t, db.Create(&artifact).Error)
	rev := store.ResearchArtifactRevision{
		ArtifactID: artifact.ID, Revision: 1, PayloadJSON: datatypes.JSON(raw),
		Origin: "user", Status: store.ResearchArtifactRevisionCandidate, CreatedBy: uid,
	}
	require.NoError(t, db.Create(&rev).Error)
	for _, dep := range deps {
		dep.ProjectID, dep.ArtifactID, dep.Revision = projectID, artifact.ID, 1
		dep.ID = store.MustNewUUID()
		require.NoError(t, db.Create(&dep).Error)
	}
	return artifact, rev
}

func planPayload() map[string]any {
	return map[string]any{"sections": []map[string]any{{
		"id": "s1", "title": "引言", "responsibility": "提出研究问题", "order": 1,
	}}}
}

func reviewPayload(planID store.UUID, planRevision int) map[string]any {
	return map[string]any{"input": map[string]any{
		"planArtifactId": planID.String(), "planRevision": planRevision,
		"normsAvailable": false,
	}}
}

func markCount(t *testing.T, db *gorm.DB, where string, args ...any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&store.ContentRecheckMark{}).Where(where, args...).Count(&count).Error)
	return count
}

// 删除资料：依赖它的产物修订（document / source_version 两种 kind）、证据、
// 有来源记录的 wiki 页全部标为待复查；跨章节检查报告只级联依据了受影响计划版本的那一份。
func TestMarkSourceDeletedCoversAllTargets(t *testing.T) {
	_, db, uid, _, pid := setup(t)
	fixture := seedDocument(t, db, pid, uid)

	plan, planRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactChapterPlan, planPayload(), []store.ResearchArtifactDependency{
		{Kind: store.ResearchDependencyDocument, RefID: fixture.Doc.ID},
		{Kind: store.ResearchDependencySourceVersion, RefID: fixture.Version.ID},
	})
	reviewCurrent, reviewCurRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactChapterReview,
		reviewPayload(plan.ID, planRev.Revision), []store.ResearchArtifactDependency{
			{Kind: store.ResearchDependencyArtifact, RefID: plan.ID},
		})
	_, reviewOldRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactChapterReview,
		reviewPayload(plan.ID, 99), []store.ResearchArtifactDependency{
			{Kind: store.ResearchDependencyArtifact, RefID: plan.ID},
		})
	// 不依赖该资料的产物不应被标记。
	_, unrelatedRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactLiteratureNote,
		map[string]any{"title": "无关记录"}, nil)

	evidence := store.AgentEvidence{
		ID: store.MustNewUUID(), RunID: store.MustNewUUID(),
		SourceType: "document", SourceID: fixture.Doc.ID, SourceName: "论文.pdf",
		SourceVersion: ptr("hash"), ChunkID: ptr("doc:0"), Quote: ptr("原文摘录"),
	}
	require.NoError(t, db.Create(&evidence).Error)

	page := rag.WikiPage{
		ID: store.MustNewUUID(), ProjectID: pid, Title: "注意力机制综述", Content: "内容",
	}
	require.NoError(t, db.Create(&page).Error)
	require.NoError(t, db.Create(&store.AgentWikiSource{
		ID: store.MustNewUUID(), WikiPageID: page.ID,
		SourceType: "document", SourceID: fixture.Doc.ID,
		SourceVersion: ptr("hash"), ChunkID: ptr("doc:0"),
	}).Error)
	// 无来源记录的页不假装可追溯。
	untraced := rag.WikiPage{ID: store.MustNewUUID(), ProjectID: pid, Title: "无来源页", Content: "内容"}
	require.NoError(t, db.Create(&untraced).Error)

	change := SourceChange{
		Kind: store.RecheckTriggerSourceDeleted, TriggerID: fixture.Doc.ID,
		DocumentID: fixture.Doc.ID, SourceID: &fixture.Source.ID,
		Reason: "资料《论文.pdf》已删除",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return MarkSourceChanged(context.Background(), tx, pid, change)
	}))

	assert.Equal(t, int64(1), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetArtifactRevision, planRev.ID), "计划修订必须被标记")
	assert.Equal(t, int64(1), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetArtifactRevision, reviewCurRev.ID))
	assert.Equal(t, int64(0), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetArtifactRevision, reviewOldRev.ID), "依据另一版计划的报告不得误标")
	assert.Equal(t, int64(0), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetArtifactRevision, unrelatedRev.ID), "无关产物不得误标")
	assert.Equal(t, int64(1), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetEvidence, evidence.ID))
	assert.Equal(t, int64(1), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetWikiPage, page.ID))
	assert.Equal(t, int64(0), markCount(t, db, "target_type = ? AND target_id = ?",
		store.RecheckTargetWikiPage, untraced.ID))

	var stale rag.WikiPage
	require.NoError(t, db.First(&stale, "id = ?", page.ID).Error)
	assert.NotNil(t, stale.StaleAt, "有来源记录的 wiki 页要置 stale_at")
	_ = reviewCurrent

	// 幂等：同一变更再执行一次不产生重复标记。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return MarkSourceChanged(context.Background(), tx, pid, change)
	}))
	assert.Equal(t, int64(4), markCount(t, db, "project_id = ?", pid))
}

// 来源出新版本：指向旧版本的依赖要标，已指向新版本的不标；文档级依赖保守标记。
func TestMarkSourceNewVersionOnlyMarksOldVersions(t *testing.T) {
	_, db, uid, _, pid := setup(t)
	fixture := seedDocument(t, db, pid, uid)
	newVersion := store.SourceVersion{
		SourceID: fixture.Source.ID, Revision: 2, S3Key: "k2", ContentSHA256: "def", SizeBytes: 2,
	}
	require.NoError(t, db.Create(&newVersion).Error)

	_, oldRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactLiteratureNote,
		map[string]any{"title": "旧版本阅读记录"}, []store.ResearchArtifactDependency{
			{Kind: store.ResearchDependencySourceVersion, RefID: fixture.Version.ID},
		})
	_, newRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactLiteratureNote,
		map[string]any{"title": "新版本阅读记录"}, []store.ResearchArtifactDependency{
			{Kind: store.ResearchDependencySourceVersion, RefID: newVersion.ID},
		})
	_, docRev := seedArtifact(t, db, pid, uid, store.ResearchArtifactLiteratureNote,
		map[string]any{"title": "文档级依赖"}, []store.ResearchArtifactDependency{
			{Kind: store.ResearchDependencyDocument, RefID: fixture.Doc.ID},
		})

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return MarkSourceChanged(context.Background(), tx, pid, SourceChange{
			Kind: store.RecheckTriggerSourceNewVersion, TriggerID: newVersion.ID,
			DocumentID: fixture.Doc.ID, SourceID: &fixture.Source.ID, NewRevision: 2,
			Reason: "资料《论文.pdf》已出新版本",
		})
	}))

	assert.Equal(t, int64(1), markCount(t, db, "target_id = ?", oldRev.ID), "旧版本依赖必须标记")
	assert.Equal(t, int64(0), markCount(t, db, "target_id = ?", newRev.ID), "新版本依赖不得标记")
	assert.Equal(t, int64(1), markCount(t, db, "target_id = ?", docRev.ID), "文档级依赖按文档保守标记")
}

// 列表与处理接口：幂等（同方式 200 / 不同方式 409）、跨项目与越权 404。
func TestListAndResolveHTTP(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	fixture := seedDocument(t, db, pid, uid)
	seedArtifact(t, db, pid, uid, store.ResearchArtifactChapterPlan, planPayload(), []store.ResearchArtifactDependency{
		{Kind: store.ResearchDependencyDocument, RefID: fixture.Doc.ID},
	})
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return MarkSourceChanged(context.Background(), tx, pid, SourceChange{
			Kind: store.RecheckTriggerSourceDeleted, TriggerID: fixture.Doc.ID,
			DocumentID: fixture.Doc.ID, SourceID: &fixture.Source.ID, Reason: "资料《论文.pdf》已删除",
		})
	}))

	cfg := &config.Settings{SecretKey: "t"}
	api := httpapi.New(nil)
	Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	otherToken, err := auth.CreateAccessToken(cfg.SecretKey, other.String(), 30)
	require.NoError(t, err)

	do := func(bearer, method, path string, body any) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			rdr = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	base := httpapi.APIVersion + "/projects/" + pid.String() + "/recheck-marks"
	listed := do(token, http.MethodGet, base, nil)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), `"targetType":"artifact_revision"`)
	assert.Contains(t, listed.Body.String(), `"targetTitle":"章节计划（1 节）"`)

	assert.Equal(t, http.StatusNotFound, do(otherToken, http.MethodGet, base, nil).Code, "非项目所有者不能读取")
	assert.Equal(t, http.StatusBadRequest, do(token, http.MethodGet, base+"?status=bogus", nil).Code)

	var marks []store.ContentRecheckMark
	require.NoError(t, db.Where("project_id = ?", pid).Find(&marks).Error)
	require.NotEmpty(t, marks)
	markID := marks[0].ID
	resolvePath := base + "/" + markID.String() + "/resolve"

	assert.Equal(t, http.StatusBadRequest, do(token, http.MethodPost, resolvePath, map[string]any{"resolution": "bogus"}).Code)
	first := do(token, http.MethodPost, resolvePath, map[string]any{"resolution": store.RecheckResolutionRechecked})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"resolution":"rechecked"`)
	// 同 resolution 幂等 200，不同 409。
	assert.Equal(t, http.StatusOK, do(token, http.MethodPost, resolvePath, map[string]any{"resolution": store.RecheckResolutionRechecked}).Code)
	assert.Equal(t, http.StatusConflict, do(token, http.MethodPost, resolvePath, map[string]any{"resolution": store.RecheckResolutionDismissed}).Code)
	// 跨项目路径访问已有 mark → 404。
	assert.Equal(t, http.StatusNotFound, do(token, http.MethodPost,
		httpapi.APIVersion+"/projects/"+store.MustNewUUID().String()+"/recheck-marks/"+markID.String()+"/resolve",
		map[string]any{"resolution": store.RecheckResolutionDismissed}).Code)
}

func ptr(value string) *string { return &value }
