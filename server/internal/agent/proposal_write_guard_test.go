package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// 写稿限制的执行级证据：真实 Run + 真实 FilesService + 本地 scripted 模型。
// 断言模型调用 propose_edit 后稿件不变，只有用户 apply 才写入；
// 建议产出后稿件被改动，旧建议 apply 必须 409 且转 stale，不得覆盖新内容。
func TestProposeEditNeverWritesBeforeUserApproval(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/write-guard.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	uid, pid, cid := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "write-guard", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "WG", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	require.NoError(t, db.Create(&store.AgentConversation{ID: cid, ProjectID: pid, UserID: uid, Title: "t"}).Error)

	ctx := context.Background()
	stg := testutil.NewMemStorage()
	files, err := latex.NewFilesService(db, stg)
	require.NoError(t, err)
	_, err = files.CreateFile(ctx, pid.UUID, "main.tex", "ABC DEF", uid.UUID)
	require.NoError(t, err)

	// 脚本化模型：奇数调用产出 propose_edit，偶数调用给出终答；每条建议一次 Run。
	var modelCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := modelCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		write := func(v any) {
			b, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
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
		writeText := func(text string) {
			write(map[string]any{"id": "ct", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0,
					"delta": map[string]any{"role": "assistant", "content": text}}}})
			write(map[string]any{"id": "ct", "object": "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{},
					"finish_reason": "stop"}}})
		}
		switch n {
		case 1:
			writeTool("p1", "propose_edit",
				`{"path":"main.tex","old_text":"ABC","new_text":"XYZ","reason":"表达更清晰"}`)
		case 3:
			writeTool("p2", "propose_edit",
				`{"path":"main.tex","old_text":"XYZ","new_text":"QQQ","reason":"再改一次"}`)
		default:
			writeText("已给出修改建议，等待你确认后应用。")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	t.Cleanup(fake.Close)

	cfg := &config.Settings{SecretKey: "t", AgentAPIKey: "k", AgentBaseURL: fake.URL, AgentModel: "m", AgentAllowPrivateProviderURLs: true}
	rg, err := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, err)
	svc := agent.NewService(db, stg, files, cfg, rg)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = svc.Close(closeCtx)
	})
	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	content := func() string {
		raw, _, gerr := files.GetFileRaw(ctx, pid.UUID, "main.tex", uid.UUID)
		require.NoError(t, gerr)
		return string(raw)
	}

	runsPath := "/api/v1/agent/conversations/" + cid.String() + "/runs"
	runOne := createdRunID(t, do(http.MethodPost, runsPath,
		`{"content":"帮我润色","expectedThreadVersion":0}`))
	assertRunTerminal(t, db, runOne, agent.RunCompleted)

	assert.Equal(t, "ABC DEF", content(), "未经用户确认，模型不得改稿")

	var call store.AgentToolCall
	require.NoError(t, db.Where("run_id = ? AND tool_name = ?", runOne, "propose_edit").First(&call).Error)
	assert.Equal(t, agent.ToolAwaiting, call.State, "建议类工具必须停在待审批")
	var first store.AgentChangeProposal
	require.NoError(t, db.Where("run_id = ?", runOne).First(&first).Error)
	assert.Equal(t, agent.ProposalPending, first.Status)

	rec := do(http.MethodPost, "/api/v1/agent/change-proposals/"+first.ID.String()+"/apply", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "XYZ DEF", content(), "用户确认后才写入")
	require.NoError(t, db.Where("id = ?", first.ID).First(&first).Error)
	assert.Equal(t, agent.ProposalApplied, first.Status)
	require.NoError(t, db.Where("id = ?", call.ID).First(&call).Error)
	assert.Equal(t, agent.ToolApproved, call.State)

	// 第二条建议产出后用户先改了稿件：apply 必须 409 且建议转 stale，不得覆盖。
	var conv store.AgentConversation
	require.NoError(t, db.Where("id = ?", cid).First(&conv).Error)
	runTwo := createdRunID(t, do(http.MethodPost, runsPath,
		fmt.Sprintf(`{"content":"再润色一次","expectedThreadVersion":%d}`, conv.Version)))
	assertRunTerminal(t, db, runTwo, agent.RunCompleted)

	var second store.AgentChangeProposal
	require.NoError(t, db.Where("run_id = ?", runTwo).First(&second).Error)
	assert.Equal(t, agent.ProposalPending, second.Status)

	_, err = files.UpdateFile(ctx, pid.UUID, "main.tex", "XYZ DEF 用户补充", uid.UUID)
	require.NoError(t, err)

	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+second.ID.String()+"/apply", "")
	assert.Equal(t, http.StatusConflict, rec.Code, "基准漂移后旧建议必须被拒")
	assert.Equal(t, "XYZ DEF 用户补充", content(), "旧建议不得覆盖用户修改")
	require.NoError(t, db.First(&second, "id = ?", second.ID).Error)
	assert.Equal(t, agent.ProposalStale, second.Status)
}
