package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestReconcileOrphanRunsFailsRunsFromPreviousProcess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/reconcile.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	old := time.Now().UTC().Add(-time.Hour)
	orphan := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(), AgentKey: AgentKeyMain,
		Status: RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v",
		StartedAt: &old, LastSequence: 1,
	}
	fresh := time.Now().UTC().Add(time.Hour)
	live := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(), AgentKey: AgentKeyMain,
		Status: RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v",
		StartedAt: &fresh,
	}
	require.NoError(t, db.Create(&orphan).Error)
	require.NoError(t, db.Create(&live).Error)
	// 崩溃场景：事件已写到 seq=2，但 last_sequence 仍是 1。
	require.NoError(t, db.Create(&store.AgentEvent{
		ID: store.MustNewUUID(), ConversationID: orphan.ConversationID,
		RunID: orphan.ID, Sequence: 2, EventType: evtTextDelta,
	}).Error)

	svc := &Service{DB: db, startedAt: time.Now().UTC()}
	require.NoError(t, svc.ReconcileOrphanRuns(context.Background()))

	var got store.AgentRun
	require.NoError(t, db.First(&got, "id = ?", orphan.ID).Error)
	assert.Equal(t, RunFailed, got.Status)
	assert.NotNil(t, got.CompletedAt)

	var finished int64
	require.NoError(t, db.Model(&store.AgentEvent{}).
		Where("run_id = ? AND event_type = ?", orphan.ID, evtRunFinished).Count(&finished).Error)
	assert.EqualValues(t, 1, finished, "订阅端必须收到终止事件")

	// 被杀掉的进程没有物化助手消息，收敛时要补一条带 error part 的消息，
	// 否则刷新后用户只看到自己的提问、看不到中断原因。
	require.NotNil(t, got.OutputMessageID)
	var msg store.AgentMessage
	require.NoError(t, db.First(&msg, "id = ?", *got.OutputMessageID).Error)
	assert.Equal(t, RunFailed, msg.Status)
	var errParts int64
	require.NoError(t, db.Model(&store.AgentMessagePart{}).
		Where("message_id = ? AND type = ?", msg.ID, PartError).Count(&errParts).Error)
	assert.EqualValues(t, 1, errParts, "中断原因必须出现在转录里")

	got = store.AgentRun{}
	require.NoError(t, db.First(&got, "id = ?", live.ID).Error)
	assert.Equal(t, RunRunning, got.Status, "本进程启动后创建的 Run 不得被误杀")
}

func TestReconcilePreservesRootTranscriptWithoutExposingChild(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/reconcile-partial.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	old := time.Now().UTC().Add(-time.Hour)
	parent := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(), AgentKey: AgentKeyMain,
		Status: RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v", StartedAt: &old,
	}
	child := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: parent.ConversationID, ParentRunID: &parent.ID,
		AgentKey: "search_references", Status: RunRunning, ModelProvider: "p", ModelName: "m",
		PromptVersion: "v", StartedAt: &old,
	}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&child).Error)
	executing := store.AgentToolCall{ID: store.MustNewUUID(), RunID: parent.ID, Seq: 1, ToolName: "read_latex_file", State: ToolExecuting}
	awaiting := store.AgentToolCall{ID: store.MustNewUUID(), RunID: parent.ID, Seq: 2, ToolName: "propose_edit", State: ToolAwaiting}
	childTool := store.AgentToolCall{ID: store.MustNewUUID(), RunID: child.ID, Seq: 1, ToolName: "rag_search", State: ToolExecuting}
	require.NoError(t, db.Create(&[]store.AgentToolCall{executing, awaiting, childTool}).Error)
	require.NoError(t, db.Create(&[]store.AgentEvent{
		{ID: store.MustNewUUID(), ConversationID: parent.ConversationID, RunID: parent.ID,
			Sequence: 1, EventType: evtTextDelta, PayloadJSON: mustJSON(map[string]string{"id": "text", "delta": "已生成内容"})},
		{ID: store.MustNewUUID(), ConversationID: parent.ConversationID, RunID: parent.ID,
			Sequence: 2, EventType: evtActivity, PayloadJSON: mustJSON(map[string]string{"runId": child.ID.String(), "title": "检索", "status": RunRunning})},
		{ID: store.MustNewUUID(), ConversationID: parent.ConversationID, RunID: child.ID,
			Sequence: 1, EventType: evtTextDelta, PayloadJSON: mustJSON(map[string]string{"id": "text", "delta": "子任务内部内容"})},
	}).Error)

	svc := &Service{DB: db, startedAt: time.Now().UTC()}
	require.NoError(t, svc.ReconcileOrphanRuns(context.Background()))
	require.NoError(t, db.First(&parent, "id = ?", parent.ID).Error)
	require.NotNil(t, parent.OutputMessageID)
	var parts []store.AgentMessagePart
	require.NoError(t, db.Where("message_id = ?", *parent.OutputMessageID).Order("ordinal").Find(&parts).Error)
	require.Len(t, parts, 3)
	assert.Equal(t, PartText, parts[0].Type)
	require.NotNil(t, parts[0].TextContent)
	assert.Equal(t, "已生成内容", *parts[0].TextContent)
	assert.Equal(t, PartActivity, parts[1].Type)
	assert.Equal(t, RunFailed, parts[1].Status)
	assert.JSONEq(t, string(mustJSON(map[string]string{"runId": child.ID.String(), "title": "检索", "status": RunFailed})), string(parts[1].ContentJSON))
	assert.Equal(t, PartError, parts[2].Type)
	require.NotNil(t, parts[2].TextContent)
	assert.Equal(t, reconcileInterrupted, *parts[2].TextContent)

	require.NoError(t, db.First(&child, "id = ?", child.ID).Error)
	assert.Equal(t, RunFailed, child.Status)
	assert.Nil(t, child.OutputMessageID, "子任务内部内容不能成为会话消息")

	require.NoError(t, db.First(&executing, "id = ?", executing.ID).Error)
	require.NoError(t, db.First(&awaiting, "id = ?", awaiting.ID).Error)
	require.NoError(t, db.First(&childTool, "id = ?", childTool.ID).Error)
	assert.Equal(t, ToolFailed, executing.State)
	assert.NotNil(t, executing.CompletedAt)
	assert.Equal(t, ToolAwaiting, awaiting.State)
	assert.Equal(t, ToolFailed, childTool.State)

	require.NoError(t, svc.ReconcileOrphanRuns(context.Background()))
	var count int64
	require.NoError(t, db.Model(&store.AgentMessage{}).Where("run_id = ?", parent.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "重复收敛不得创建第二条回复")
}

func TestSettleReconciledActivitiesUsesChildTerminalStatus(t *testing.T) {
	child := store.AgentRun{ID: store.MustNewUUID(), Status: RunCompleted}
	parts := []transcriptPart{
		{Type: PartActivity, Status: RunRunning, Content: mustJSON(map[string]string{"runId": child.ID.String(), "status": RunRunning})},
		{Type: PartActivity, Status: RunCompleted, Content: mustJSON(map[string]string{"runId": child.ID.String(), "status": RunCompleted})},
	}

	settled := settleReconciledActivities(parts, []store.AgentRun{child})
	require.Len(t, settled, 1)
	assert.Equal(t, RunCompleted, settled[0].Status)
}

func TestReconcilePreservesLongUTF8AnswerWithinTextLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/reconcile-long.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:mysql-text-limit", func(tx *gorm.DB) {
		if part, ok := tx.Statement.Dest.(*store.AgentMessagePart); ok && part.TextContent != nil && len(*part.TextContent) > 65535 {
			_ = tx.AddError(errors.New("TEXT exceeds MySQL byte limit"))
		}
	}))

	old := time.Now().UTC().Add(-time.Hour)
	run := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(), AgentKey: AgentKeyMain,
		Status: RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v", StartedAt: &old,
	}
	text := strings.Repeat("论", 25000) + "finish"
	require.NoError(t, db.Create(&run).Error)
	require.NoError(t, db.Create(&store.AgentEvent{
		ID: store.MustNewUUID(), ConversationID: run.ConversationID, RunID: run.ID, Sequence: 1,
		EventType: evtTextDelta, PayloadJSON: mustJSON(map[string]string{"id": "text", "delta": text}),
	}).Error)

	svc := &Service{DB: db, startedAt: time.Now().UTC()}
	require.NoError(t, svc.ReconcileOrphanRuns(context.Background()))
	require.NoError(t, db.First(&run, "id = ?", run.ID).Error)
	require.NotNil(t, run.OutputMessageID)
	var parts []store.AgentMessagePart
	require.NoError(t, db.Where("message_id = ?", *run.OutputMessageID).Order("ordinal").Find(&parts).Error)
	var recovered strings.Builder
	for _, part := range parts {
		if part.Type != PartText {
			continue
		}
		require.NotNil(t, part.TextContent)
		assert.LessOrEqual(t, len(*part.TextContent), 65535)
		assert.True(t, utf8.ValidString(*part.TextContent))
		recovered.WriteString(*part.TextContent)
	}
	assert.Equal(t, text, recovered.String())
}
