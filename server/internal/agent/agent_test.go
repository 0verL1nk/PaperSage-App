package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约：完整 ReAct 循环。假 OpenAI 服务第 1 轮返回 tool_call，
// 第 2 轮返回终答；断言 SSE 事件序列 + 会话/消息落库（云端状态）。

// fakeOpenAI：OpenAI 兼容 SSE /chat/completions，脚本化两轮响应
// （eino openai 组件默认流式请求，须返回 text/event-stream 增量块）。
func fakeOpenAI(t *testing.T) *httptest.Server {
	t.Helper()
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl := w.(http.Flusher)
		write := func(v any) {
			b, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		if calls == 1 {
			// 工具调用增量块
			delta := map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"index": 0, "id": "call_1", "type": "function",
					"function": map[string]any{"name": "list_latex_files", "arguments": "{}"},
				}},
			}
			write(map[string]any{"id": "cmpl-1", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
			write(map[string]any{"id": "cmpl-1", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}})
		} else {
			write(map[string]any{"id": "cmpl-2", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0,
					"delta":         map[string]any{"role": "assistant", "content": "你的项目有 main.tex，结构完整。"},
					"finish_reason": nil}}})
			write(map[string]any{"id": "cmpl-2", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
}

func TestAgentReActLoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	// 用户 + 项目 + 一个 latex 文件
	uid := store.MustNewUUID()
	user := store.User{Username: "agenttester", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}
	require.NoError(t, db.Create(&user).Error)
	pid := store.MustNewUUID()
	proj := store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}
	require.NoError(t, db.Create(&proj).Error)
	s3Key := "projects/" + pid.String() + "/latex/main.tex"
	hash := "h1"
	f := store.LatexFile{ProjectID: pid, FilePath: "main.tex",
		S3Key: &s3Key, SizeBytes: 5, ContentHash: &hash,
		BaseModel: store.BaseModel{ID: store.MustNewUUID()}}
	require.NoError(t, db.Create(&f).Error)

	// 假模型服务 + 装配
	fake := fakeOpenAI(t)
	cfg := &config.Settings{
		SecretKey: "t", AgentAPIKey: "test-key",
		AgentBaseURL: fake.URL, AgentModel: "test-model",
	}
	mem := testutil.NewMemStorage()
	require.NoError(t, mem.Put(context.Background(), s3Key, []byte("hello")))

	latexSvc, err := latex.NewService(nil, cfg)
	require.NoError(t, err)
	queue := latex.NewQueue(db, nil, latexSvc, cfg)
	agentSvc := agent.NewService(db, mem, queue, cfg)
	authSvc := auth.NewService(db, cfg, nil)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)

	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	// ── 发起对话（SSE）──
	body, _ := json.Marshal(map[string]string{"content": "看看我的论文文件"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+pid.String()+"/agent/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

	// 解析 SSE 事件
	var evts []map[string]any
	sc := bufio.NewScanner(rec.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			var e map[string]any
			require.NoError(t, json.Unmarshal([]byte(line[6:]), &e))
			evts = append(evts, e)
		}
	}
	kinds := make([]string, 0, len(evts))
	for _, e := range evts {
		kinds = append(kinds, e["type"].(string))
	}
	assert.Contains(t, kinds, "tool_call")
	assert.Contains(t, kinds, "tool_result")
	assert.Contains(t, kinds, "delta")
	assert.Equal(t, "done", kinds[len(kinds)-1])

	var convID string
	var sawFile, sawFinal bool
	for _, e := range evts {
		if e["type"] == "meta" {
			convID = e["conversationId"].(string)
		}
		if e["type"] == "tool_result" && strings.Contains(e["result"].(string), "main.tex") {
			sawFile = true
		}
		if e["type"] == "delta" && strings.Contains(e["delta"].(string), "main.tex") {
			sawFinal = true
		}
	}
	assert.True(t, sawFile, "工具结果应含 main.tex")
	assert.True(t, sawFinal, "终答应引用文件")
	require.NotEmpty(t, convID)

	// ── 落库断言（云端会话状态）──
	var nMsg int64
	convHex := strings.ReplaceAll(convID, "-", "")
	require.NoError(t, db.Table("agent_messages").
		Where("conversation_id = ?", convHex).Count(&nMsg).Error)
	assert.Equal(t, int64(2), nMsg, "user + assistant 两条消息")

	// ── 会话列表 + 历史消息 API ──
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+pid.String()+"/agent/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "看看我的论文文件", "会话标题")

	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/agent/conversations/"+convID+"/messages", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "结构完整")
}

func TestAgentNoModelConfigured(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent2.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid := store.MustNewUUID()
	user := store.User{Username: "nomodel", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}
	require.NoError(t, db.Create(&user).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P2", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	cfg := &config.Settings{SecretKey: "t"} // 无 AGENT_*，用户也无设置
	latexSvc, _ := latex.NewService(nil, cfg)
	queue := latex.NewQueue(db, nil, latexSvc, cfg)
	agentSvc := agent.NewService(db, testutil.NewMemStorage(), queue, cfg)
	authSvc := auth.NewService(db, cfg, nil)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)

	token, _ := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	body, _ := json.Marshal(map[string]string{"content": "hi"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+pid.String()+"/agent/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "尚未配置")
}

var _ = gin.H{}
