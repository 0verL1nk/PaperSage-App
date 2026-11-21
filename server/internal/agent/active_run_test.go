package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 刷新后前端要能找回进行中的 Run；否则界面看起来像“没开始”，用户会重复发送。
func TestActiveRunLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/active.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "active-a", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	convID := store.MustNewUUID()
	doneConv := store.MustNewUUID()
	foreignConv := store.MustNewUUID()
	require.NoError(t, db.Create(&[]store.AgentConversation{
		{ID: convID, ProjectID: pid, UserID: uid, Title: "running"},
		{ID: doneConv, ProjectID: pid, UserID: uid, Title: "done"},
		{ID: foreignConv, ProjectID: pid, UserID: other, Title: "theirs"},
	}).Error)

	running := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: convID, AgentKey: agent.AgentKeyMain,
		Status: agent.RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v",
		CreatedAt: time.Now().UTC().Add(-time.Minute), LastSequence: 7,
	}
	finished := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: doneConv, AgentKey: agent.AgentKeyMain,
		Status: agent.RunCompleted, ModelProvider: "p", ModelName: "m", PromptVersion: "v",
	}
	require.NoError(t, db.Create(&running).Error)
	child := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: convID, ParentRunID: &running.ID,
		AgentKey: "search_references", Status: agent.RunRunning,
		ModelProvider: "p", ModelName: "m", PromptVersion: "v", CreatedAt: time.Now().UTC().Add(time.Second),
	}
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&finished).Error)

	cfg := &config.Settings{SecretKey: "t"}
	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	svc := agent.NewService(db, testutil.NewMemStorage(), nil, cfg, rgStore)
	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	get := func(conv store.UUID) (int, string, int64) {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/agent/conversations/"+conv.String()+"/runs/active", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		var body struct {
			Data struct {
				RunID        *string `json:"runId"`
				LastSequence int64   `json:"lastSequence"`
			} `json:"data"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if body.Data.RunID == nil {
			return rec.Code, "", body.Data.LastSequence
		}
		return rec.Code, *body.Data.RunID, body.Data.LastSequence
	}

	code, runID, lastSequence := get(convID)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, running.ID.String(), runID)
	assert.EqualValues(t, 7, lastSequence)

	require.NoError(t, db.Model(&running).Update("status", agent.RunCompleted).Error)
	code, runID, _ = get(convID)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, runID, "只剩子 Run 时不得恢复它")

	code, runID, _ = get(doneConv)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, runID, "已结束的会话不应返回进行中的 Run")

	code, _, _ = get(foreignConv)
	assert.Equal(t, http.StatusNotFound, code, "不能读到别人的会话状态")
}
