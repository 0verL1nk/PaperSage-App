package agent_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// fakeCompletion 模拟 OpenAI 兼容的非流式 /chat/completions，
// 记录最后一次请求体，供断言 prompt 是否装配了三源上下文。
func fakeCompletion(t *testing.T, content string) (*httptest.Server, func() string) {
	t.Helper()
	var mu sync.Mutex
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastBody = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cmpl-1", "object": "chat.completion", "created": 1, "model": "test-model",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return lastBody
	}
}

type experimentTestEnv struct {
	db     *gorm.DB
	api    *httpapi.API
	cfg    *config.Settings
	userID store.UUID
	projID store.UUID
	token  string
}

func newExperimentTestEnv(t *testing.T, baseURL string, withModel bool) *experimentTestEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/experiment.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{
		Username: "experimenter", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid},
	}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{
		Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid},
	}).Error)

	cfg := &config.Settings{SecretKey: "test-secret"}
	if withModel {
		cfg.AgentAPIKey = "test-key"
		cfg.AgentBaseURL = baseURL
		cfg.AgentModel = "test-model"
		cfg.AgentAllowPrivateProviderURLs = true
	}

	svc := agent.NewService(db, testutil.NewMemStorage(), nil, cfg, nil)
	authSvc := auth.NewService(db, cfg, nil)
	router := httpapi.New(nil)
	agent.Mount(router.Group(httpapi.APIVersion), authSvc.AuthRequired(), svc)

	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	return &experimentTestEnv{db: db, api: router, cfg: cfg, userID: uid, projID: pid, token: token}
}

func (e *experimentTestEnv) post(t *testing.T, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+e.projID.String()+"/agent/experiment-analysis", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.api.ServeHTTP(rec, req)
	return rec
}

func validExperimentBody() map[string]any {
	return map[string]any{
		"table": map[string]any{
			"headers": []string{"方法", "准确率 (%)", "F1"},
			"rows":    [][]string{{"Baseline", "88.1", "0.842"}, {"Ours", "92.5", "0.881"}},
			"caption": "不同方法在测试集上的性能对比",
			"label":   "tab:results",
			"bold":    []string{"none", "max", "max"},
		},
		"language": "zh",
		"focus":    "重点说明提升幅度",
	}
}

const exactAnalysisJSON = `{
	"paragraphs": [{
		"text": "本文方法准确率达到 92.5%，较基线 88.1% 提升 4.4 个百分点。",
		"citations": [
			{"row": 1, "column": 1, "value": "92.5"},
			{"row": 0, "column": 1, "value": "88.1"}
		]
	}],
	"norm_refs": []
}`

func TestExperimentAnalysisReturnsVerifiedDraft(t *testing.T) {
	fake, lastBody := fakeCompletion(t, exactAnalysisJSON)
	env := newExperimentTestEnv(t, fake.URL, true)

	rec := env.post(t, env.token, validExperimentBody())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Paragraphs []struct {
				Text      string `json:"text"`
				Citations []struct {
					Row         int    `json:"row"`
					Column      int    `json:"column"`
					Value       string `json:"value"`
					RowLabel    string `json:"rowLabel"`
					ColumnLabel string `json:"columnLabel"`
				} `json:"citations"`
			} `json:"paragraphs"`
			Context struct {
				NormsCount int      `json:"normsCount"`
				PlanStatus string   `json:"planStatus"`
				Warnings   []string `json:"warnings"`
			} `json:"context"`
			Model struct {
				Name   string `json:"name"`
				Source string `json:"source"`
			} `json:"model"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 200, envelope.Code)
	require.Len(t, envelope.Data.Paragraphs, 1)
	citations := envelope.Data.Paragraphs[0].Citations
	require.Len(t, citations, 2)
	assert.Equal(t, "Ours", citations[0].RowLabel)
	assert.Equal(t, "准确率 (%)", citations[0].ColumnLabel)
	assert.Equal(t, "92.5", citations[0].Value)
	assert.Equal(t, "server-env", envelope.Data.Model.Source)
	assert.Contains(t, rec.Body.String(), `"normRefs":[]`, "空规范引用必须是数组，不能是 null")
	assert.Equal(t, "missing", envelope.Data.Context.PlanStatus, "没有章节计划时必须如实说明")
	assert.Contains(t, envelope.Data.Context.Warnings, "项目还没有可用的章节计划，分析未对齐章节职责")

	// 三源上下文：规范不可用声明 + 蓝图为空 + 表格矩阵 + 用户的补充要求。
	prompt := lastBody()
	assert.Contains(t, prompt, "实验表格")
	assert.Contains(t, prompt, "row 0: Baseline | 88.1 | 0.842")
	assert.Contains(t, prompt, "本次没有可用的项目规范")
	// 请求体经 encoding/json 序列化，尖括号会转义为 </>，这里只断言标签名。
	assert.Contains(t, prompt, "user_focus")
	assert.Contains(t, prompt, "不得编造任何实验数值")
}

func TestExperimentAnalysisRejectsFabricatedCitation(t *testing.T) {
	fabricated := `{"paragraphs":[{"text":"本文方法准确率达到 93.1%。","citations":[{"row":1,"column":1,"value":"93.1"}]}]}`
	fake, _ := fakeCompletion(t, fabricated)
	env := newExperimentTestEnv(t, fake.URL, true)

	rec := env.post(t, env.token, validExperimentBody())
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "引用值")
	assert.Contains(t, rec.Body.String(), "第 2 行第 2 列是「92.5」", "错误信息要指出真实单元格，便于用户核对")
}

func TestExperimentAnalysisValidatesRequest(t *testing.T) {
	env := newExperimentTestEnv(t, "", false)
	body := validExperimentBody()
	table := body["table"].(map[string]any)
	table["rows"] = [][]string{{"只有一列"}}

	rec := env.post(t, env.token, body)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "表头是 3 列")
}

func TestExperimentAnalysisRequiresOwnership(t *testing.T) {
	env := newExperimentTestEnv(t, "", false)
	other := store.MustNewUUID()
	require.NoError(t, env.db.Create(&store.User{
		Username: "other", PasswordHash: "x", BaseModel: store.BaseModel{ID: other},
	}).Error)
	otherToken, err := auth.CreateAccessToken(env.cfg.SecretKey, other.String(), 30)
	require.NoError(t, err)

	rec := env.post(t, otherToken, validExperimentBody())
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestExperimentAnalysisWithoutModelIsExplicit(t *testing.T) {
	env := newExperimentTestEnv(t, "", false)
	rec := env.post(t, env.token, validExperimentBody())
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "尚未配置 AI 模型")
}
