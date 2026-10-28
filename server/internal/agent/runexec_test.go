package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestCloseRacesAdmissionAndOtherClosers(t *testing.T) {
	for range 20 {
		svc := NewService(nil, nil, nil, nil, nil)
		start := make(chan struct{})
		var requests, closers sync.WaitGroup
		for range 32 {
			requests.Go(func() {
				<-start
				_, release, accepted := svc.acceptRun(context.Background())
				if accepted {
					release()
				}
			})
		}
		for range 8 {
			closers.Go(func() {
				<-start
				assert.NoError(t, svc.Close(context.Background()))
			})
		}
		close(start)
		requests.Wait()
		closers.Wait()
		_, _, accepted := svc.acceptRun(context.Background())
		require.False(t, accepted)
	}
}

func TestTerminalTransactionIsIdempotentAndKeepsApproval(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/terminal.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	svc := NewService(db, nil, nil, nil, nil)
	conv := store.AgentConversation{ID: store.MustNewUUID(), ProjectID: store.MustNewUUID(), UserID: store.MustNewUUID()}
	require.NoError(t, db.Create(&conv).Error)
	root := store.AgentRun{ID: store.MustNewUUID(), ConversationID: conv.ID, AgentKey: AgentKeyMain, Status: RunRunning}
	require.NoError(t, db.Create(&root).Error)
	child, err := svc.createChildRun(context.Background(), runContext{RunID: root.ID, ConversationID: conv.ID}, "child")
	require.NoError(t, err)
	awaiting := store.AgentToolCall{ID: store.MustNewUUID(), RunID: root.ID, Seq: 1, ToolName: "propose_edit", State: ToolAwaiting}
	executing := store.AgentToolCall{ID: store.MustNewUUID(), RunID: child.ID, Seq: 1, ToolName: "rag_search", State: ToolExecuting}
	require.NoError(t, db.Create(&awaiting).Error)
	require.NoError(t, db.Create(&executing).Error)
	ctx, cancel := context.WithTimeout(context.Background(), runFinalizeTimeout)
	defer cancel()
	rc := runContext{RunID: root.ID, ConversationID: conv.ID}
	// A committed child is owned by the parent even if its creation result was lost.
	require.NoError(t, svc.finalizeRun(ctx, rc, "partial", nil, RunCancelled, "cancelled"))
	require.NoError(t, svc.finalizeRun(ctx, rc, "late completion", nil, RunCompleted, ""))
	require.NoError(t, db.First(&root, "id = ?", root.ID).Error)
	require.NoError(t, db.First(child, "id = ?", child.ID).Error)
	assert.Equal(t, RunCancelled, root.Status)
	assert.Equal(t, RunCancelled, child.Status)
	require.NoError(t, db.First(&awaiting, "id = ?", awaiting.ID).Error)
	require.NoError(t, db.First(&executing, "id = ?", executing.ID).Error)
	assert.Equal(t, ToolAwaiting, awaiting.State, "shutdown must not decide approval for the user")
	assert.Equal(t, ToolFailed, executing.State)
	var events []store.AgentEvent
	require.NoError(t, db.Where("run_id = ?", root.ID).Order("sequence").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, evtActivity, events[0].EventType)
	assert.Equal(t, evtRunFinished, events[1].EventType)
	var messages []store.AgentMessage
	require.NoError(t, db.Where("run_id = ?", root.ID).Find(&messages).Error)
	require.Len(t, messages, 1)
	assert.Equal(t, RunCancelled, messages[0].Status)
	assert.WithinDuration(t, time.Now().UTC(), *root.CompletedAt, time.Second)

	t.Run("assistant insert failure still terminates the Run", func(t *testing.T) {
		failure := errors.New("message store unavailable")
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-answer", func(tx *gorm.DB) {
			if tx.Statement.Table == "agent_messages" {
				_ = tx.AddError(failure)
			}
		}))
		defer func() { require.NoError(t, db.Callback().Create().Remove("test:fail-answer")) }()
		root := store.AgentRun{ID: store.MustNewUUID(), ConversationID: conv.ID, AgentKey: AgentKeyMain, Status: RunRunning}
		require.NoError(t, db.Create(&root).Error)
		rc := runContext{RunID: root.ID, ConversationID: conv.ID}
		require.ErrorIs(t, svc.finalizeRun(ctx, rc, "answer", nil, RunCompleted, ""), failure)
		require.NoError(t, db.First(&root, "id = ?", root.ID).Error)
		assert.Equal(t, RunFailed, root.Status)
		assert.NotNil(t, root.CompletedAt)
		var event store.AgentEvent
		require.NoError(t, db.Where("run_id = ? AND event_type = ?", root.ID, evtRunFinished).First(&event).Error)
		assert.JSONEq(t, `{"status":"failed"}`, string(event.PayloadJSON))
	})
}
