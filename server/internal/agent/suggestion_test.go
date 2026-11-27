package agent_test

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
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

const suggestionParagraph = "人工智能正在改变科研写作方式，但通用写作助手常常给出缺少依据的空泛建议。"

func TestSuggestionRunGatesAndUndo(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/suggestion.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	uid, pid := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "suggestion", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "SG", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	ctx := context.Background()
	stg := testutil.NewMemStorage()
	files, err := latex.NewFilesService(db, stg)
	require.NoError(t, err)
	_, err = files.CreateFile(ctx, pid.UUID, "main.tex", suggestionParagraph+"\n\n正文。", uid.UUID)
	require.NoError(t, err)
	_, err = files.CreateFile(ctx, pid.UUID, "undo.tex", "ABC DEF", uid.UUID)
	require.NoError(t, err)

	// 假模型：奇数次调用跳过本段，偶数次给出终答；每次 Run 重复这一对。
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		write := func(v any) {
			b, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
		if n%2 == 1 {
			tc := map[string]any{"id": fmt.Sprintf("skip%d", n), "type": "function",
				"function": map[string]any{"name": "skip_suggestion", "arguments": `{"reason":"表达已清晰"}`}}
			write(map[string]any{"id": "c", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{tc}}},
			}})
			write(map[string]any{"id": "c", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"},
			}})
		} else {
			write(map[string]any{"id": "t", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "本段暂无建议。"}},
			}})
			write(map[string]any{"id": "t", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"},
			}})
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(fake.Close)

	cfg := &config.Settings{
		SecretKey: "t", AgentAPIKey: "k", AgentBaseURL: fake.URL, AgentModel: "m",
		AgentAllowPrivateProviderURLs: true, AgentSuggestionEnabled: true,
		AgentSuggestionMaxIterations: 4,
	}
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

	suggestionsPath := "/api/v1/projects/" + pid.String() + "/agent/suggestions"
	// 段落过短：不建 Run。
	rec := do(http.MethodPost, suggestionsPath, `{"filePath":"main.tex","paragraphText":"太短","trigger":"manual"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "too_short")

	// 段落不在稿件里：必须 409，不能拿旧内容给建议。
	rec = do(http.MethodPost, suggestionsPath,
		`{"filePath":"main.tex","paragraphText":"这段文字并不在当前稿件中，但长度已经超过二十个字符。","trigger":"manual"}`)
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	// 正常手动检查：隐藏会话 + trigger=manual，Run 跑完但不产生 pending 建议。
	body, err := json.Marshal(map[string]any{
		"filePath": "main.tex", "paragraphText": suggestionParagraph,
		"sectionPath": "绪论 / 研究背景", "trigger": "manual", "clientRequestId": "req-1",
	})
	require.NoError(t, err)
	rec = do(http.MethodPost, suggestionsPath, string(body))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	runID := createdRunID(t, rec)
	assertRunTerminal(t, db, runID, agent.RunCompleted)

	var run store.AgentRun
	require.NoError(t, db.Where("id = ?", runID).First(&run).Error)
	assert.Equal(t, agent.SuggestionTriggerManual, run.Trigger)
	assert.Equal(t, agent.SuggestionAgentKey, run.AgentKey)
	assert.NotEmpty(t, run.FocusJSON)

	var conv store.AgentConversation
	require.NoError(t, db.Where("id = ?", run.ConversationID).First(&conv).Error)
	assert.Equal(t, "suggestion", conv.Kind)
	rec = do(http.MethodGet, "/api/v1/projects/"+pid.String()+"/agent/conversations", "")
	assert.NotContains(t, rec.Body.String(), conv.ID.String(), "建议会话不得出现在会话列表")

	// 同段已有 pending：再次检查直接跳过。
	pending := store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: pid, FilePath: "main.tex",
		BaseContentHash: md5Of(suggestionParagraph), OldText: "a", NewText: "b",
		ParagraphHash: sha256Of(suggestionParagraph), Status: "pending",
	}
	require.NoError(t, db.Create(&pending).Error)
	// 换一个 clientRequestId，避免命中幂等重放而绕过门禁。
	body2, err := json.Marshal(map[string]any{
		"filePath": "main.tex", "paragraphText": suggestionParagraph,
		"sectionPath": "绪论 / 研究背景", "trigger": "manual", "clientRequestId": "req-2",
	})
	require.NoError(t, err)
	rec = do(http.MethodPost, suggestionsPath, string(body2))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "pending")

	// 撤销：应用后 new_text 唯一存在 → 恢复原文；再撤销 409。
	undo := store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: pid, FilePath: "undo.tex",
		BaseContentHash: md5Of("ABC DEF"), OldText: "ABC", NewText: "XYZ", Status: "pending",
	}
	require.NoError(t, db.Create(&undo).Error)
	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+undo.ID.String()+"/apply", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	raw, _, err := files.GetFileRaw(ctx, pid.UUID, "undo.tex", uid.UUID)
	require.NoError(t, err)
	assert.Equal(t, "XYZ DEF", string(raw))

	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+undo.ID.String()+"/undo", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	raw, _, err = files.GetFileRaw(ctx, pid.UUID, "undo.tex", uid.UUID)
	require.NoError(t, err)
	assert.Equal(t, "ABC DEF", string(raw))
	require.NoError(t, db.Where("id = ?", undo.ID).First(&undo).Error)
	assert.Equal(t, agent.ProposalReverted, undo.Status)
	assert.NotNil(t, undo.RevertedAt)

	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+undo.ID.String()+"/undo", "")
	assert.Equal(t, http.StatusConflict, rec.Code, "重复撤销必须被拒")

	// 拒绝原因落库，供质量评测。
	reject := store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: pid, FilePath: "undo.tex",
		BaseContentHash: md5Of("ABC DEF"), OldText: "ABC", NewText: "QQQ", Status: "pending",
	}
	require.NoError(t, db.Create(&reject).Error)
	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+reject.ID.String()+"/reject",
		`{"reasonCode":"too_vague","note":"没有落到具体句"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, db.Where("id = ?", reject.ID).First(&reject).Error)
	assert.Equal(t, agent.ProposalRejected, reject.Status)
	assert.Equal(t, "too_vague", reject.RejectReason)
	require.NotNil(t, reject.RejectNote)
	assert.Equal(t, "没有落到具体句", *reject.RejectNote)

	// 非法原因枚举被拒，不能污染评测数据。
	reject2 := store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: pid, FilePath: "undo.tex",
		BaseContentHash: md5Of("ABC DEF"), OldText: "ABC", NewText: "EEE", Status: "pending",
	}
	require.NoError(t, db.Create(&reject2).Error)
	rec = do(http.MethodPost, "/api/v1/agent/change-proposals/"+reject2.ID.String()+"/reject",
		`{"reasonCode":"made_up"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func md5Of(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
