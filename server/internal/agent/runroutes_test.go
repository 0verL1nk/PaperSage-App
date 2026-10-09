package agent_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// sseParts 解析 UI Message Stream：跳过终止标记 [DONE]（它不是 JSON part）。
func sseParts(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[6:]
		if payload == "[DONE]" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &e))
		out = append(out, e)
	}
	return out
}

func partTypes(parts []map[string]any) []string {
	types := make([]string, 0, len(parts))
	for _, p := range parts {
		types = append(types, p["type"].(string))
	}
	return types
}

// Run 生命周期契约：创建 → 订阅 → 幂等 → 乐观锁 → 重试 → 取消。
func TestRunLifecycleContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/runs.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})

	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "run-user", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "RP", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	convID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{
		ID: convID, ProjectID: pid, UserID: uid, Title: "t"}).Error)

	fake := fakeOpenAI(t)
	cfg := &config.Settings{SecretKey: "t", AgentAPIKey: "k",
		AgentBaseURL: fake.URL, AgentModel: "test-model"}
	latexSvc, _ := latex.NewService(nil, cfg)
	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	svc := agent.NewService(db, testutil.NewMemStorage(), nil,
		latex.NewQueue(db, nil, latexSvc, cfg), cfg, rgStore)

	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != "" {
			rdr = bytes.NewReader([]byte(body))
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	type createResp struct {
		Data struct {
			TurnID        string `json:"turnId"`
			RunID         string `json:"runId"`
			UserMessageID string `json:"userMessageId"`
			StreamURL     string `json:"streamUrl"`
			ThreadVersion int64  `json:"threadVersion"`
		} `json:"data"`
	}
	runsPath := "/api/v1/agent/conversations/" + convID.String() + "/runs"

	// 1. 创建 Run
	rec := do(http.MethodPost, runsPath, `{"content":"看看我的文件","clientRequestId":"req-1"}`)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var created createResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.RunID)
	assert.EqualValues(t, 1, created.Data.ThreadVersion)

	// 2. 订阅：协议版本头存在，part 以 finish 收尾
	rec = do(http.MethodGet, "/api/v1/agent/runs/"+created.Data.RunID+"/events", "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "v1", rec.Header().Get("x-vercel-ai-ui-message-stream"))
	types := partTypes(sseParts(t, rec.Body))
	require.NotEmpty(t, types)
	assert.Equal(t, "finish", types[len(types)-1], "流应以 finish 收尾")
	assert.Contains(t, types, "data-conversation")

	// 2b. 工具调用已落入状态机：有序号、有幂等键、已收敛到终态
	runUUID, err := uuid.Parse(created.Data.RunID)
	require.NoError(t, err)
	var calls []store.AgentToolCall
	require.NoError(t, db.Where("run_id = ?", store.UUID{UUID: runUUID}).
		Order("seq ASC").Find(&calls).Error)
	require.NotEmpty(t, calls, "工具调用应落库")
	assert.EqualValues(t, 1, calls[0].Seq, "序号从 1 开始且单调")
	assert.Equal(t, "list_latex_files", calls[0].ToolName)
	assert.Equal(t, "read", calls[0].RiskLevel, "只读工具自动执行")
	assert.NotNil(t, calls[0].IdempotencyKey, "应记录幂等键")
	assert.Equal(t, "succeeded", calls[0].State)

	// 3. 幂等：同一 clientRequestId 复用同一 Turn 与同一 Run，不重复执行
	rec = do(http.MethodPost, runsPath, `{"content":"看看我的文件","clientRequestId":"req-1"}`)
	require.Equal(t, 200, rec.Code)
	var again createResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	assert.Equal(t, created.Data.TurnID, again.Data.TurnID, "应复用同一 Turn")
	assert.Equal(t, created.Data.RunID, again.Data.RunID, "应复用同一 Run（否则幂等键名存实亡）")

	// 4. 乐观锁：版本不匹配 → 409 且带服务端最新版本
	rec = do(http.MethodPost, runsPath, `{"content":"x","clientRequestId":"req-2","expectedThreadVersion":99}`)
	assert.Equal(t, 409, rec.Code)
	assert.Contains(t, rec.Body.String(), "threadVersion")

	// 5. 重试：复用同一 Turn 新建 Run
	rec = do(http.MethodPost, "/api/v1/agent/runs/"+created.Data.RunID+"/retry", "")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "runId")

	// 6. 取消：终态后再取消不报错（cancelled=false）
	rec = do(http.MethodPost, "/api/v1/agent/runs/"+created.Data.RunID+"/cancel", "")
	require.Equal(t, 200, rec.Code)

	// 7. 越权：不存在/不属于自己的 Run → 404
	rec = do(http.MethodGet, "/api/v1/agent/runs/00000000-0000-0000-0000-000000000000/events", "")
	assert.Equal(t, 404, rec.Code)

	// 8. Run evidence is owner-scoped and includes the exact stored excerpt version.
	sourceID := store.MustNewUUID()
	chunkID := "document:" + sourceID.String() + ":2"
	sourceVersion := strings.Repeat("a", 64)
	locator := "chunk:2"
	quote := "a persisted source excerpt"
	evidence := store.AgentEvidence{ID: store.MustNewUUID(), RunID: store.UUID{UUID: runUUID},
		SourceType: "document", SourceID: sourceID, SourceVersion: &sourceVersion,
		ChunkID: &chunkID, Locator: &locator, Quote: &quote}
	require.NoError(t, db.Create(&evidence).Error)
	rec = do(http.MethodGet, "/api/v1/agent/runs/"+created.Data.RunID+"/evidence", "")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"sourceVersion":"`+sourceVersion+`"`)
	assert.Contains(t, rec.Body.String(), `"quote":"`+quote+`"`)

	strangerID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "run-stranger", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: strangerID}}).Error)
	strangerToken, err := auth.CreateAccessToken(cfg.SecretKey, strangerID.String(), 30)
	require.NoError(t, err)
	strangerReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent/runs/"+created.Data.RunID+"/evidence", nil)
	strangerReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerRec := httptest.NewRecorder()
	api.ServeHTTP(strangerRec, strangerReq)
	assert.Equal(t, 404, strangerRec.Code)
	assert.NotContains(t, strangerRec.Body.String(), quote)
}
