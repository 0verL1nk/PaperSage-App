package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// Agentic search 契约：主 agent 调 search_references（子 agent），
// 子 agent 调 rag_search 命中文献块并 wiki_write 沉淀知识页，
// 最终主 agent 输出带出处的综合摘要。
//
// 假 OpenAI 服务按调用序号脚本化：
//
//	#1 主 agent → tool_call: search_references
//	#2 子 agent → tool_call: rag_search {query:"对比学习"}
//	#3 子 agent → tool_call: wiki_write（沉淀）
//	#4 子 agent → 终答（返回给主 agent 作为工具结果）
//	#5 主 agent → 终答（带出处）
func TestAgenticSearchSubagent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agentic.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	require.NoError(t, db.AutoMigrate(&rag.IndexedChunk{}, &rag.WikiPage{}))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "agentic", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "AP", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	// 文献已入库并索引（模拟 confirm 钩子的产物）
	rg, err := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, err)
	docID := store.MustNewUUID()
	_, err = rg.IndexSource(context.Background(), pid, "document", docID,
		"contrastive-learning.pdf",
		"对比学习 contrastive learning 通过正负样本对学习表示。SimCLR 与 MoCo 是两大代表工作。")
	require.NoError(t, err)

	var call atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := call.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		write := func(v any) {
			b, _ := json.Marshal(v)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
			fl.Flush()
		}
		writeTool := func(id, name, args string) {
			tc := map[string]any{"id": id, "type": "function",
				"function": map[string]any{"name": name, "arguments": args}}
			write(map[string]any{"id": "c" + id, "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0,
					"delta": map[string]any{"role": "assistant", "tool_calls": []any{tc}}}}})
			write(map[string]any{"id": "c" + id, "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{},
					"finish_reason": "tool_calls"}}})
		}
		writeText := func(id, text string) {
			write(map[string]any{"id": id, "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0,
					"delta": map[string]any{"role": "assistant", "content": text}}}})
			write(map[string]any{"id": id, "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{},
					"finish_reason": "stop"}}})
		}
		switch n {
		case 1:
			writeTool("t1", "search_references", `{"task":"调研对比学习"}`)
		case 2:
			writeTool("t2", "rag_search", `{"query":"对比学习"}`)
		case 3:
			writeTool("t3", "wiki_write", `{"title":"对比学习笔记","content":"对比学习=正负样本对。来源: contrastive-learning.pdf"}`)
		case 4:
			writeText("c4", "检索完成：对比学习通过正负样本对学习表示 [来源: contrastive-learning.pdf]。")
		case 5:
			writeText("c5", "根据资料库调研：对比学习通过正负样本对学习表示，SimCLR/MoCo 为代表 [来源: contrastive-learning.pdf]。")
		default:
			writeText("cx", "done")
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	}))
	defer fake.Close()

	cfg := &config.Settings{SecretKey: "t",
		AgentAPIKey: "k", AgentBaseURL: fake.URL, AgentModel: "m"}
	latexSvc, _ := latex.NewService(nil, cfg)
	queue := latex.NewQueue(db, nil, latexSvc, cfg)
	svc := agent.NewService(db, testutil.NewMemStorage(), queue, cfg, rg)
	authSvc := auth.NewService(db, cfg, nil)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	agent.Mount(v1, authSvc.AuthRequired(), svc)

	token, _ := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	body, _ := json.Marshal(map[string]string{"content": "调研一下对比学习"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+pid.String()+"/agent/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())

	var kinds []string
	sc := bufio.NewScanner(rec.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var sawSearchTool, sawRagHit, sawFinal bool
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line[6:]), &e))
		kinds = append(kinds, toStr(e["type"]))
		if e["type"] == "tool_call" && e["tool"] == "search_references" {
			sawSearchTool = true
		}
		if e["type"] == "tool_result" && strings.Contains(toStr(e["result"]), "对比学习") {
			sawRagHit = true
		}
		if e["type"] == "delta" && strings.Contains(toStr(e["delta"]), "来源") {
			sawFinal = true
		}
	}
	assert.True(t, sawSearchTool, "主 agent 应调用 search_references")
	assert.True(t, sawRagHit, "子 agent 的 rag 命中应作为 tool_result 流出")
	assert.True(t, sawFinal, "终答应带出处")
	assert.Equal(t, "done", kinds[len(kinds)-1])

	// wiki 沉淀落库
	var pages int64
	require.NoError(t, db.Table("agent_wiki_pages").
		Where("project_id = ?", pid).Count(&pages).Error)
	assert.Equal(t, int64(1), pages, "子 agent 应沉淀 1 页 wiki")
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
