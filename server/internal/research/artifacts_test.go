package research

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setup(t *testing.T) (*Service, *gorm.DB, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/research.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "research-a", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "research-b", PasswordHash: "x", BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)
	return NewService(db), db, uid, other, pid
}

func seedSource(t *testing.T, db *gorm.DB, projectID store.UUID) store.UUID {
	t.Helper()
	src := store.ProjectSource{ProjectID: projectID, Kind: "document", CurrentRevision: 1}
	require.NoError(t, db.Create(&src).Error)
	ver := store.SourceVersion{SourceID: src.ID, Revision: 1, S3Key: "k", ContentSHA256: strings.Repeat("a", 64), SizeBytes: 1}
	require.NoError(t, db.Create(&ver).Error)
	return ver.ID
}

func notePayload(source store.UUID) json.RawMessage {
	field := func(text string, evidence bool) map[string]any {
		f := map[string]any{"text": text, "status": "reported"}
		if evidence {
			f["evidence"] = []map[string]any{{"sourceVersionId": source.String()}}
		}
		return f
	}
	raw, _ := json.Marshal(map[string]any{
		"source":   map[string]any{"sourceVersionId": source.String()},
		"question": field("研究问题", true), "method": field("方法", true),
		"objects": field("对象", true), "conditions": field("条件", true),
		"results": field("结果", true), "limitations": field("限制", true),
	})
	return raw
}

func TestCreateListAndIdempotency(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	source := seedSource(t, db, pid)
	ctx := context.Background()
	clientID := "req-1"
	created, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source),
		Origin: "agent", ClientRequestID: &clientID,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, created.CurrentRevision)
	require.NotNil(t, created.Current)
	assert.Equal(t, store.ResearchArtifactRevisionCandidate, created.Current.Status)
	assert.Nil(t, created.AdoptedRevision)

	again, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source), ClientRequestID: &clientID,
	})
	require.NoError(t, err)
	assert.Equal(t, created.ID, again.ID, "同一 clientRequestId 不得重复建产物")

	items, err := svc.List(ctx, uid, pid, store.ResearchArtifactLiteratureNote)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestReportedFieldRequiresEvidence(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	source := seedSource(t, db, pid)
	_, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source),
	})
	require.NoError(t, err)

	bad := notePayload(source)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(bad, &payload))
	delete(payload["results"].(map[string]any), "evidence")
	raw, _ := json.Marshal(payload)
	_, err = svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: raw,
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
}

func TestAdoptAndSupersede(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactChapterPlan, Payload: planPayload()})
	require.NoError(t, err)
	artifactID := store.UUID{UUID: created.ID}

	adopted, err := svc.Adopt(ctx, uid, pid, artifactID, decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err)
	require.NotNil(t, adopted.AdoptedRevision)
	assert.Equal(t, 1, *adopted.AdoptedRevision)

	withCandidate, err := svc.AddRevision(ctx, uid, pid, artifactID, revisionRequest{
		ExpectedRevision: 1, Payload: planPayload(), Origin: "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, withCandidate.CurrentRevision)
	assert.Equal(t, 1, *withCandidate.AdoptedRevision, "新候选不得自动替换已采用计划")

	after, err := svc.Adopt(ctx, uid, pid, artifactID, decisionRequest{Revision: 2, ExpectedRevision: 2})
	require.NoError(t, err)
	assert.Equal(t, 2, *after.AdoptedRevision)
	statuses := map[int]string{}
	for _, rev := range after.Revisions {
		statuses[rev.Revision] = rev.Status
	}
	assert.Equal(t, store.ResearchArtifactRevisionSuperseded, statuses[1])
	assert.Equal(t, store.ResearchArtifactRevisionAdopted, statuses[2])
}

func TestRevisionConflictAndReject(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactChapterPlan, Payload: planPayload()})
	require.NoError(t, err)
	artifactID := store.UUID{UUID: created.ID}

	_, err = svc.AddRevision(ctx, uid, pid, artifactID, revisionRequest{ExpectedRevision: 9, Payload: planPayload()})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 409, apiErr.Code)

	rejected, err := svc.Reject(ctx, uid, pid, artifactID, decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err)
	assert.Equal(t, store.ResearchArtifactRevisionRejected, rejected.Revisions[0].Status)
}

func TestCrossProjectIsolation(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	source := seedSource(t, db, pid)
	ctx := context.Background()
	_, err := svc.Create(ctx, other, pid, createRequest{Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source)})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Code)

	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source)})
	require.NoError(t, err)
	_, err = svc.Get(ctx, other, pid, store.UUID{UUID: created.ID})
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Code)
}

func TestForeignEvidenceRejected(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	_ = other
	foreignPID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "Other", OwnerID: uid, BaseModel: store.BaseModel{ID: foreignPID}}).Error)
	foreignSource := seedSource(t, db, foreignPID)
	_, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(foreignSource),
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
}

func planPayload() json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"sections": []map[string]any{
			{"id": "s1", "title": "引言", "responsibility": "提出问题", "order": 1},
			{"id": "s2", "title": "方法", "responsibility": "说明方法", "order": 2, "dependencies": []string{"s1"}, "filePath": "chapters/method.tex"},
		},
	})
	return raw
}

func seedLatexFile(t *testing.T, db *gorm.DB, projectID store.UUID, path string) {
	t.Helper()
	require.NoError(t, db.Create(&store.LatexFile{ProjectID: projectID, FilePath: path, SizeBytes: 1}).Error)
}

func seedAdoptedPlan(t *testing.T, svc *Service, uid, pid store.UUID) store.UUID {
	t.Helper()
	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactChapterPlan, Payload: planPayload()})
	require.NoError(t, err)
	_, err = svc.Adopt(ctx, uid, pid, store.UUID{UUID: created.ID}, decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err)
	return store.UUID{UUID: created.ID}
}

func reviewPayload(planID store.UUID, revision int) map[string]any {
	return map[string]any{
		"input": map[string]any{
			"planArtifactId": planID.String(), "planRevision": revision, "planStatus": "adopted",
			"normsAvailable": false, "normsNote": "学术规范库尚未接入",
		},
		"dimensions": []map[string]any{
			{"key": "goal_conclusion", "status": "findings"},
			{"key": "evidence_support", "status": "insufficient", "note": "缺少可用资料原文"},
			{"key": "cross_section", "status": "checked"},
			{"key": "norm_compliance", "status": "not_checked", "note": "规范库尚未接入"},
		},
		"findings": []map[string]any{{
			"id": "f1", "dimension": "goal_conclusion", "severity": "high",
			"sectionId": "s2", "sectionTitle": "方法", "filePath": "chapters/method.tex", "quote": "样本 120 份",
			"problem": "方法写 120 份，结论写 200 份", "nextStep": "统一两处样本量",
			"basis": []map[string]any{
				{"kind": "text", "ref": "chapters/method.tex", "filePath": "chapters/method.tex", "quote": "样本 120 份"},
				{"kind": "plan", "ref": "s2", "quote": "说明方法"},
			},
			"confidence": "high",
		}},
		"sourcesRead": []map[string]any{{"kind": "latex", "name": "chapters/method.tex"}},
	}
}

func TestChapterReviewPayloadValidation(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	seedLatexFile(t, db, pid, "chapters/method.tex")
	planID := seedAdoptedPlan(t, svc, uid, pid)
	raw, err := json.Marshal(reviewPayload(planID, 1))
	require.NoError(t, err)

	created, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactChapterReview, Payload: raw,
	})
	require.NoError(t, err)
	assert.Equal(t, store.ResearchArtifactChapterReview, created.Kind)
	assert.Equal(t, 1, created.CurrentRevision)
}

func TestChapterReviewRejectsIncompleteOrFakePass(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	seedLatexFile(t, db, pid, "chapters/method.tex")
	planID := seedAdoptedPlan(t, svc, uid, pid)

	cases := map[string]func(map[string]any){
		"缺少维度": func(p map[string]any) {
			p["dimensions"] = []map[string]any{{"key": "goal_conclusion", "status": "checked"}}
		},
		"维度状态非法": func(p map[string]any) {
			p["dimensions"].([]map[string]any)[0]["status"] = "bogus"
		},
		"证据不足未写原因": func(p map[string]any) {
			delete(p["dimensions"].([]map[string]any)[1], "note")
		},
		"规范库不可用却声称已检查": func(p map[string]any) {
			p["dimensions"].([]map[string]any)[3] = map[string]any{"key": "norm_compliance", "status": "checked"}
		},
		"定位文件不存在": func(p map[string]any) {
			p["findings"].([]map[string]any)[0]["filePath"] = "chapters/missing.tex"
		},
		"章节不在计划中": func(p map[string]any) {
			p["findings"].([]map[string]any)[0]["sectionId"] = "s9"
		},
		"问题无依据": func(p map[string]any) {
			p["findings"].([]map[string]any)[0]["basis"] = []map[string]any{}
		},
		"维度声称发现问题但无问题": func(p map[string]any) {
			p["dimensions"].([]map[string]any)[1] = map[string]any{"key": "evidence_support", "status": "findings"}
		},
		"维度声称已检查但有问题": func(p map[string]any) {
			p["dimensions"].([]map[string]any)[0] = map[string]any{"key": "goal_conclusion", "status": "checked"}
		},
		"定位文件与计划不一致": func(p map[string]any) {
			seedLatexFile(t, db, pid, "chapters/other.tex")
			p["findings"].([]map[string]any)[0]["filePath"] = "chapters/other.tex"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			payload := reviewPayload(planID, 1)
			mutate(payload)
			raw, err := json.Marshal(payload)
			require.NoError(t, err)
			_, err = svc.Create(context.Background(), uid, pid, createRequest{
				Kind: store.ResearchArtifactChapterReview, Payload: raw,
			})
			var apiErr *httpapi.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 400, apiErr.Code)
		})
	}
}

func TestChapterReviewRejectsForeignPlan(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	seedLatexFile(t, db, pid, "chapters/method.tex")
	foreignProject := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "Other", OwnerID: uid, BaseModel: store.BaseModel{ID: foreignProject}}).Error)
	seedLatexFile(t, db, foreignProject, "chapters/method.tex")
	foreignPlan := seedAdoptedPlan(t, svc, uid, foreignProject)

	raw, err := json.Marshal(reviewPayload(foreignPlan, 1))
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactChapterReview, Payload: raw,
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
}

// 回归：产物路由 URL 只带 artifact_id，handler 需自行解析归属项目。
// 修复前这 5 条路径都因缺 :project_id 返回 404。
func TestArtifactRoutesResolveProject(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	cfg := &config.Settings{SecretKey: "t"}
	api := httpapi.New(nil)
	Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			rdr = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	// create 路由原本就带 :project_id，用它造前置产物。
	created := do(http.MethodPost, httpapi.APIVersion+"/projects/"+pid.String()+"/research-artifacts",
		map[string]any{"kind": store.ResearchArtifactChapterPlan, "payload": planPayload()})
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.ID)
	base := httpapi.APIVersion + "/research-artifacts/" + envelope.Data.ID

	// 1. 新增修订
	added := do(http.MethodPost, base+"/revisions",
		map[string]any{"expectedRevision": 1, "payload": planPayload(), "origin": "agent"})
	require.Equal(t, http.StatusOK, added.Code, added.Body.String())

	// 2. 读修订
	got := do(http.MethodGet, base+"/revisions/2", nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	assert.Contains(t, got.Body.String(), `"revision":2`)

	// 3. 采用
	adopted := do(http.MethodPost, base+"/adopt", map[string]any{"revision": 2, "expectedRevision": 2})
	require.Equal(t, http.StatusOK, adopted.Code, adopted.Body.String())
	assert.Contains(t, adopted.Body.String(), `"adoptedRevision":2`)

	// 4. 拒绝（先加一版候选再拒）
	require.Equal(t, http.StatusOK, do(http.MethodPost, base+"/revisions",
		map[string]any{"expectedRevision": 2, "payload": planPayload(), "origin": "agent"}).Code)
	rejected := do(http.MethodPost, base+"/reject", map[string]any{"revision": 3, "expectedRevision": 3})
	require.Equal(t, http.StatusOK, rejected.Code, rejected.Body.String())
	assert.Contains(t, rejected.Body.String(), `"status":"rejected"`)

	// 5. 删除：之后列表为空
	require.Equal(t, http.StatusOK, do(http.MethodDelete, base, nil).Code)
	listed := do(http.MethodGet, httpapi.APIVersion+"/projects/"+pid.String()+"/research-artifacts", nil)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), `"data":[]`)
}

// 回归：检查页用 ?kind=chapter_review 读取历史报告。kind 白名单曾漏掉
// chapter_review，线上每次进入检查页都会返回 400「不支持的产物类型」。
func TestListChapterReviewKindIsAccepted(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	cfg := &config.Settings{SecretKey: "t"}
	api := httpapi.New(nil)
	Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, httpapi.APIVersion+"/projects/"+pid.String()+"/research-artifacts?kind=chapter_review", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "不支持的产物类型")
}

// 规范依据必须逐条命中本次生效清单：模型编造一条编号要被拒，而不是落进报告。
func TestChapterReviewRejectsUnknownNormBasis(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	seedLatexFile(t, db, pid, "chapters/method.tex")
	planID := seedAdoptedPlan(t, svc, uid, pid)

	payload := reviewPayload(planID, 1)
	payload["input"].(map[string]any)["normsAvailable"] = true
	payload["input"].(map[string]any)["normsNote"] = ""
	payload["input"].(map[string]any)["norms"] = []map[string]any{{
		"id": "entry-1", "code": "GBT7714", "version": 2,
		"title": "类型标识", "rule": "用 [J] 标注。", "source": "GB/T 7714—2015",
	}}
	finding := payload["findings"].([]map[string]any)[0]
	finding["basis"] = []map[string]any{{"kind": "norm", "ref": "N-FAKE@v9", "quote": "编造的条款"}}

	create := createRequest{Kind: store.ResearchArtifactChapterReview, Payload: mustJSON(t, payload)}
	if _, err := svc.Create(context.Background(), uid, pid, create); err == nil {
		t.Fatal("引用不存在的规范条款必须被拒绝")
	}

	finding["basis"] = []map[string]any{{"kind": "norm", "ref": "N-GBT7714@v2", "quote": "用 [J] 标注。"}}
	if _, err := svc.Create(context.Background(), uid, pid, createRequest{Kind: store.ResearchArtifactChapterReview, Payload: mustJSON(t, payload)}); err != nil {
		t.Fatalf("命中清单的规范依据应被接受：%v", err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func seedDocument(t *testing.T, db *gorm.DB, projectID, ownerID store.UUID, sourceID *store.UUID) store.UUID {
	t.Helper()
	doc := store.Document{
		Name: "paper.pdf", Type: "application/pdf", Status: "uploaded",
		S3Key: "k/paper", ProjectID: projectID, OwnerID: ownerID, SourceID: sourceID,
	}
	require.NoError(t, db.Create(&doc).Error)
	return doc.ID
}

// documentNotePayload 用 documentId（而不是 sourceVersionId）引用依据，
// 与 rag_search 返回 source_id 后模型按工具说明产出的写法一致。
func documentNotePayload(document store.UUID) json.RawMessage {
	field := func(text string, evidence bool) map[string]any {
		f := map[string]any{"text": text, "status": "reported"}
		if evidence {
			f["evidence"] = []map[string]any{{"documentId": document.String()}}
		}
		return f
	}
	raw, _ := json.Marshal(map[string]any{
		"source":   map[string]any{"documentId": document.String()},
		"question": field("研究问题", true), "method": field("方法", true),
		"objects": field("对象", true), "conditions": field("条件", true),
		"results": field("结果", true), "limitations": field("限制", true),
	})
	return raw
}

// 回归：documentId 证据必须记录**实际来源版本**（最大 revision），
// 而不是把文档 ID 记成 source_version——后者会让 adopt 按版本表校验时必然 400。
func TestDocumentEvidenceRecordsResolvedSourceVersion(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	version := seedSource(t, db, pid)
	var v1 store.SourceVersion
	require.NoError(t, db.Where("id = ?", version).First(&v1).Error)
	v2 := store.SourceVersion{SourceID: v1.SourceID, Revision: 2, S3Key: "k2",
		ContentSHA256: strings.Repeat("b", 64), SizeBytes: 1}
	require.NoError(t, db.Create(&v2).Error)
	docID := seedDocument(t, db, pid, uid, &v1.SourceID)

	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: documentNotePayload(docID),
	})
	require.NoError(t, err)

	var deps []store.ResearchArtifactDependency
	require.NoError(t, db.Where("artifact_id = ? AND revision = ?", store.UUID{UUID: created.ID}, 1).Find(&deps).Error)
	require.NotEmpty(t, deps)
	for _, dep := range deps {
		assert.Equal(t, store.ResearchDependencySourceVersion, dep.Kind,
			"有来源版本的文档必须记 source_version 依赖")
		assert.Equal(t, v2.ID, dep.RefID, "必须记录当前（最大 revision）版本")
	}

	_, err = svc.Adopt(ctx, uid, pid, store.UUID{UUID: created.ID},
		decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err, "documentId 证据的产物必须可 adopt")
}

// 没有来源版本的文档只能记 document 依赖，adopt 仍须成功。
func TestDocumentWithoutSourceVersionUsesDocumentDependency(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	docID := seedDocument(t, db, pid, uid, nil)

	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: documentNotePayload(docID),
	})
	require.NoError(t, err)

	var deps []store.ResearchArtifactDependency
	require.NoError(t, db.Where("artifact_id = ? AND revision = ?", store.UUID{UUID: created.ID}, 1).Find(&deps).Error)
	require.NotEmpty(t, deps)
	for _, dep := range deps {
		assert.Equal(t, store.ResearchDependencyDocument, dep.Kind)
		assert.Equal(t, docID, dep.RefID)
	}

	_, err = svc.Adopt(ctx, uid, pid, store.UUID{UUID: created.ID},
		decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err, "document 依赖的产物必须可 adopt")
}

// 跨项目 documentId 仍须拒绝（归属校验在 validateRef）。
func TestCrossProjectDocumentEvidenceRejected(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	otherPid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "Q", OwnerID: uid,
		BaseModel: store.BaseModel{ID: otherPid}}).Error)
	docID := seedDocument(t, db, otherPid, uid, nil)

	_, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: documentNotePayload(docID),
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code, "跨项目 documentId 必须拒绝")
}
