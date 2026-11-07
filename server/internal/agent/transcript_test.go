package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestBuildTranscriptKeepsToolIdentityAndProposal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/transcript.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	convID, runID, childID := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	add := func(seq int64, run store.UUID, typ string, payload any) {
		t.Helper()
		require.NoError(t, db.Create(&store.AgentEvent{
			ID: store.MustNewUUID(), ConversationID: convID, RunID: run,
			Sequence: seq, EventType: typ, PayloadJSON: datatypes.JSON(mustJSON(payload)),
		}).Error)
	}
	add(1, runID, evtTextDelta, map[string]string{"id": "text_1", "delta": "你"})
	add(2, runID, evtToolInput, map[string]string{"toolCallId": "c1", "toolName": "list_documents", "input": `{"q":1}`})
	add(3, runID, evtToolOutput, map[string]string{"toolCallId": "c1", "output": "ok"})
	add(4, runID, evtTextDelta, map[string]string{"id": "text_1", "delta": "好"})
	add(5, runID, evtToolInput, map[string]string{"toolCallId": "c2", "toolName": "list_documents", "input": `{"q":2}`})
	add(6, runID, evtProposal, map[string]any{"id": "p1", "filePath": "main.tex", "status": "pending"})
	add(7, runID, evtRunError, map[string]string{"message": "boom"})
	// 子 agent 的 silent 事件只写子 Run，不能进入主转录。
	add(1, childID, evtTextDelta, map[string]string{"id": "text_child", "delta": "secret"})

	require.NoError(t, db.Create(&store.AgentToolCall{
		ID: store.MustNewUUID(), RunID: runID, Seq: 1, ToolName: "list_documents",
		State: ToolSucceeded, InputJSON: datatypes.JSON(`{"q":1}`),
	}).Error)
	require.NoError(t, db.Create(&store.AgentToolCall{
		ID: store.MustNewUUID(), RunID: runID, Seq: 2, ToolName: "list_documents",
		State: ToolFailed, InputJSON: datatypes.JSON(`{"q":2}`),
	}).Error)

	svc := &Service{DB: db}
	parts, err := svc.buildTranscript(context.Background(), runID, "", "")
	require.NoError(t, err)
	require.Len(t, parts, 5, "text + 两个工具 + 建议 + 错误")

	assert.Equal(t, PartText, parts[0].Type)
	require.NotNil(t, parts[0].Text)
	assert.Equal(t, "你好", *parts[0].Text, "同一 text id 的 delta 必须聚合")
	assert.NotContains(t, *parts[0].Text, "secret", "子 Run 事件不得进入主转录")

	assert.Equal(t, PartTool, parts[1].Type)
	assert.Equal(t, ToolSucceeded, parts[1].Status)
	assert.Equal(t, PartTool, parts[2].Type)
	assert.Equal(t, ToolFailed, parts[2].Status, "同名工具必须按 toolCallId 分开且保留各自状态")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(parts[1].Content, &meta))
	assert.Equal(t, "list_documents", meta["toolName"])
	assert.Equal(t, "ok", meta["output"])

	assert.Equal(t, PartProposal, parts[3].Type)
	assert.Equal(t, "pending", parts[3].Status)
	assert.Equal(t, PartError, parts[4].Type)
	require.NotNil(t, parts[4].Text)
	assert.Equal(t, "boom", *parts[4].Text)
}
