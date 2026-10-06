package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// 删除会话是软删：会话与消息不再可见，但记录必须保留——
// 后续的运行记录与验收证据要挂在这些行上（见 data-integrity-refactor D2）。
//
// 断言刻意落在「行还在」而非仅「接口读不到」：后者在退回物理删除时同样成立，
// 那样的用例守不住这个决定。
func TestDeleteConversationIsSoftDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/conv.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid, uid2 := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "conv-a", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "conv-b", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid2}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	convID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{
		ID: convID, ProjectID: pid, UserID: uid, Title: "mine"}).Error)
	msgID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentMessage{
		ID: msgID, ConversationID: convID, Role: "user", Status: "completed"}).Error)
	text := "hi"
	require.NoError(t, db.Create(&store.AgentMessagePart{
		ID: store.MustNewUUID(), MessageID: msgID, Ordinal: 0,
		Type: "text", Status: "completed", TextContent: &text}).Error)

	foreignID := store.MustNewUUID() // 归属另一个用户
	require.NoError(t, db.Create(&store.AgentConversation{
		ID: foreignID, ProjectID: pid, UserID: uid2, Title: "theirs"}).Error)

	cfg := &config.Settings{SecretKey: "t"}
	latexSvc, _ := latex.NewService(nil, cfg)
	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	agentSvc := agent.NewService(db, testutil.NewMemStorage(), nil,
		latex.NewQueue(db, nil, latexSvc, cfg), cfg, rgStore)

	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), agentSvc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	convURL := func(id store.UUID) string { return "/api/v1/agent/conversations/" + id.String() }

	msgPath := convURL(convID) + "/messages"
	require.Equal(t, 200, do(http.MethodGet, msgPath).Code, "删除前消息应可读")

	listPath := "/api/v1/projects/" + pid.String() + "/agent/conversations"
	listLen := func() int {
		rec := do(http.MethodGet, listPath)
		require.Equal(t, 200, rec.Code)
		var body struct {
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		return len(body.Data)
	}
	require.Equal(t, 1, listLen(), "删除前列表应含该会话")

	// 越权：他人会话不可删，且不得被改动
	assert.Equal(t, 404, do(http.MethodDelete, convURL(foreignID)).Code, "越权删除应 404")
	var foreign store.AgentConversation
	require.NoError(t, db.First(&foreign, "id = ?", foreignID).Error,
		"他人会话不应被软删（GORM 默认过滤已删行，能查到即说明未删）")

	require.Equal(t, 200, do(http.MethodDelete, convURL(convID)).Code)
	assert.Equal(t, 404, do(http.MethodGet, msgPath).Code, "删除后消息不可见")
	assert.Equal(t, 0, listLen(), "已删会话不应出现在列表中")

	var convCount, msgCount int64
	require.NoError(t, db.Unscoped().Model(&store.AgentConversation{}).
		Where("id = ?", convID).Count(&convCount).Error)
	require.NoError(t, db.Unscoped().Model(&store.AgentMessage{}).
		Where("conversation_id = ?", convID).Count(&msgCount).Error)
	assert.EqualValues(t, 1, convCount, "会话行应保留（软删，供后续运行记录挂载）")
	assert.EqualValues(t, 1, msgCount, "消息行应保留（软删）")

	// 仅断言行数不足以证明"软删"——行数在原样保留时同样是 1。
	// 必须读到 deleted_at，才能证明事务里的消息软删真的执行了。
	var msg store.AgentMessage
	require.NoError(t, db.Unscoped().Where("conversation_id = ?", convID).First(&msg).Error)
	assert.True(t, msg.DeletedAt.Valid, "消息应被标记 deleted_at，而非原样留在库里")

	var conv store.AgentConversation
	require.NoError(t, db.Unscoped().Where("id = ?", convID).First(&conv).Error)
	assert.True(t, conv.DeletedAt.Valid, "会话应被标记 deleted_at")

	assert.Equal(t, 404, do(http.MethodDelete, convURL(convID)).Code, "重复删除应 404")
}

// 建会话 + 立刻发 Run：这是新项目里「用户发出的第一句话」的完整路径。
//
// 曾经这条路径是断的——Run 要求会话已存在，而没有任何接口能建会话，
// 于是新项目的第一条消息在前端本地回显后无处可发（输入被清空，什么都没发生）。
func TestCreateConversationThenRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/create-conv.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "cc-a", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "cc-b", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	foreignPid := store.MustNewUUID() // 属于另一个用户的项目
	require.NoError(t, db.Create(&store.Project{Name: "Q", OwnerID: other,
		BaseModel: store.BaseModel{ID: foreignPid}}).Error)

	cfg := &config.Settings{SecretKey: "t"}
	latexSvc, _ := latex.NewService(nil, cfg)
	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	agentSvc := agent.NewService(db, testutil.NewMemStorage(), nil,
		latex.NewQueue(db, nil, latexSvc, cfg), cfg, rgStore)

	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), agentSvc)
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

	// 越权：不能在他人的项目下建会话
	assert.Equal(t, 404,
		do(http.MethodPost, "/api/v1/projects/"+foreignPid.String()+"/agent/conversations", `{}`).Code)

	// 正常建会话：标题默认，版本从 0 起（前端据此做乐观锁）
	rec := do(http.MethodPost, "/api/v1/projects/"+pid.String()+"/agent/conversations", `{"title":"你好"}`)
	require.Equal(t, 200, rec.Code)
	var created struct {
		Data struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&created))
	require.NotEmpty(t, created.Data.ID)
	assert.Equal(t, "你好", created.Data.Title)
	assert.EqualValues(t, 0, created.Data.Version)

	// 关键：新会话立刻可用于发 Run。未配置模型是 400（「尚未配置 AI 模型」），
	// 而不是 404「会话不存在」——后者正是修复前新项目的结局。
	runRec := do(http.MethodPost, "/api/v1/agent/conversations/"+created.Data.ID+"/runs",
		`{"content":"你好","clientRequestId":"req-1","expectedThreadVersion":0}`)
	assert.Equal(t, 400, runRec.Code, "应停在「未配置模型」，而不是「会话不存在」")

	// 列表里能看到它
	listRec := do(http.MethodGet, "/api/v1/projects/"+pid.String()+"/agent/conversations", "")
	require.Equal(t, 200, listRec.Code)
	var list struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(listRec.Body).Decode(&list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, created.Data.ID, list.Data[0]["id"])
}
