package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const reviewMethodText = `\section{方法}
本研究共收集 120 份有效样本。
采用问卷与访谈相结合的方式。`

// reviewMemStorage 是测试用的最小对象存储；避免在 package agent 内引入
// testutil（testutil 依赖 agent，会形成测试导入环）。
type reviewMemStorage struct{ data map[string][]byte }

func newReviewMemStorage() *reviewMemStorage {
	return &reviewMemStorage{data: map[string][]byte{}}
}

func (m *reviewMemStorage) Put(_ context.Context, key string, data []byte) error {
	m.data[key] = append([]byte(nil), data...)
	return nil
}

func (m *reviewMemStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := m.data[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (m *reviewMemStorage) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *reviewMemStorage) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (m *reviewMemStorage) List(_ context.Context, prefix string) ([]string, error) {
	keys := []string{}
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *reviewMemStorage) Exists(_ context.Context, key string) bool {
	_, ok := m.data[key]
	return ok
}

type reviewFixture struct {
	db      *gorm.DB
	stg     *reviewMemStorage
	owner   store.UUID
	project store.UUID
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/review.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	owner, project := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: owner}, Username: "review-user", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: project}, Name: "Review", OwnerID: owner}).Error)

	stg := newReviewMemStorage()
	ctx := context.Background()
	for path, content := range map[string]string{
		"main.tex":                "\\documentclass{article}\n\\begin{document}\n\\input{chapters/method}\n\\end{document}\n",
		"chapters/method.tex":     reviewMethodText,
		"chapters/conclusion.tex": "\\section{结论}\n本研究共收集 200 份有效样本。\n",
	} {
		key := storage.BuildKey(project.String(), path)
		require.NoError(t, stg.Put(ctx, key, []byte(content)))
		require.NoError(t, db.Create(&store.LatexFile{
			ProjectID: project, FilePath: path, SizeBytes: int64(len(content)), S3Key: &key,
		}).Error)
	}

	plan := `{"sections":[{"id":"s1","title":"方法","responsibility":"说明方法与样本量","order":1,"filePath":"chapters/method.tex"},{"id":"s2","title":"结论","responsibility":"总结发现","order":2,"filePath":"chapters/conclusion.tex"}]}`
	artifact := store.ResearchArtifact{
		ProjectID: project, Kind: store.ResearchArtifactChapterPlan, State: "active",
		CurrentRevision: 1, CreatedBy: owner,
	}
	require.NoError(t, db.Create(&artifact).Error)
	adopted := 1
	require.NoError(t, db.Model(&store.ResearchArtifact{}).Where("id = ?", artifact.ID).
		Update("adopted_revision", adopted).Error)
	require.NoError(t, db.Create(&store.ResearchArtifactRevision{
		ArtifactID: artifact.ID, Revision: 1, PayloadJSON: []byte(plan),
		Origin: "user", Status: store.ResearchArtifactRevisionAdopted, CreatedBy: owner,
	}).Error)

	return reviewFixture{db: db, stg: stg, owner: owner, project: project}
}

func reviewReport(quote string) map[string]any {
	return map[string]any{
		"input": map[string]any{},
		"dimensions": []map[string]any{
			{"key": "goal_conclusion", "status": "findings"},
			{"key": "evidence_support", "status": "insufficient", "note": "资料原文不足"},
			{"key": "cross_section", "status": "checked"},
			{"key": "norm_compliance", "status": "not_checked", "note": "规范库尚未接入"},
		},
		"findings": []map[string]any{{
			"id": "f1", "dimension": "goal_conclusion", "severity": "high",
			"sectionId": "s1", "filePath": "chapters/method.tex", "quote": quote,
			"problem": "方法与结论的样本量不一致", "nextStep": "统一两处样本量",
			"basis": []map[string]any{
				{"kind": "text", "ref": "chapters/method.tex", "quote": "120 份有效样本"},
				{"kind": "plan", "ref": "s1", "quote": "说明方法与样本量"},
			},
			"confidence": "high",
		}},
		"sourcesRead": []map[string]any{{"kind": "latex", "name": "模型自报的来源会被覆盖"}},
	}
}

func prepareReviewReads(t *testing.T, fixture reviewFixture) (*reviewReads, *reviewContextState) {
	t.Helper()
	ctx := context.Background()
	out, state, sources, err := buildChapterReviewContext(ctx, fixture.db, fixture.stg, norms.Unavailable{}, fixture.project, fixture.owner)
	require.NoError(t, err)
	require.NotNil(t, out.Plan)
	require.Len(t, out.Chapters, 2)
	require.False(t, out.Norms.Available)
	reads := newReviewReads()
	reads.setContext(state)
	for _, src := range sources {
		reads.record(src.Kind, src.Name)
	}
	return reads, state
}

func TestVerifyChapterReviewPayloadAnchorsQuotes(t *testing.T) {
	fixture := newReviewFixture(t)
	reads, _ := prepareReviewReads(t, fixture)
	raw, err := json.Marshal(reviewReport("本研究共收集 120 份有效样本。"))
	require.NoError(t, err)

	verified, err := verifyChapterReviewPayload(context.Background(), fixture.db, fixture.stg, fixture.project, raw, reads)
	require.NoError(t, err)
	var report research.ChapterReview
	require.NoError(t, json.Unmarshal(verified, &report))

	require.Len(t, report.Findings, 1)
	assert.True(t, report.Findings[0].AnchorVerified)
	assert.Equal(t, 2, report.Findings[0].LineStart, "行号应由服务端按原文计算")
	assert.Equal(t, 2, report.Findings[0].LineEnd)
	assert.Equal(t, "adopted", report.Input.PlanStatus)
	require.Len(t, report.Input.PlanSections, 2)
	assert.Equal(t, "方法", report.Input.PlanSections[0].Title)
	require.NotNil(t, report.Input.PlanArtifactID)
	assert.False(t, report.Input.NormsAvailable)
	assert.NotEmpty(t, report.SourcesRead)
	for _, src := range report.SourcesRead {
		assert.NotEqual(t, "模型自报的来源会被覆盖", src.Name)
	}
}

func TestVerifyChapterReviewPayloadRejectsFabrication(t *testing.T) {
	fixture := newReviewFixture(t)
	reads, _ := prepareReviewReads(t, fixture)

	raw, err := json.Marshal(reviewReport("本研究共收集 500 份有效样本。"))
	require.NoError(t, err)
	_, err = verifyChapterReviewPayload(context.Background(), fixture.db, fixture.stg, fixture.project, raw, reads)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "找不到")
}

func TestVerifyChapterReviewPayloadRequiresContext(t *testing.T) {
	fixture := newReviewFixture(t)
	raw, err := json.Marshal(reviewReport("本研究共收集 120 份有效样本。"))
	require.NoError(t, err)
	_, err = verifyChapterReviewPayload(context.Background(), fixture.db, fixture.stg, fixture.project, raw, newReviewReads())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get_chapter_review_context")
}

func TestLocateQuoteNormalizesWhitespace(t *testing.T) {
	start, end, ok := locateQuote("第一行\n样本 120 份，\n第三行", "样本   120\n份")
	require.True(t, ok)
	assert.Equal(t, 2, start)
	assert.Equal(t, 2, end)
	_, _, ok = locateQuote("只有这句话", "不存在的引文")
	assert.False(t, ok)
}
