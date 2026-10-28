package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func eventFixture(t *testing.T) (*Service, runContext) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/events.db?_busy_timeout=5000&_journal_mode=WAL"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	conv := store.AgentConversation{ID: store.MustNewUUID(), ProjectID: store.MustNewUUID(), UserID: store.MustNewUUID()}
	require.NoError(t, db.Create(&conv).Error)
	run := store.AgentRun{ID: store.MustNewUUID(), ConversationID: conv.ID, AgentKey: AgentKeyMain, Status: RunRunning}
	require.NoError(t, db.Create(&run).Error)
	return NewService(db, nil, nil, nil, nil), runContext{RunID: run.ID, ConversationID: conv.ID}
}

func TestEventInsertCancellationCannotStrandRun(t *testing.T) {
	svc, rc := eventFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inserted := false
	// The old Create committed before this callback, then its separate sequence
	// update saw the cancelled context and left a durable event ahead of the Run.
	require.NoError(t, svc.DB.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("test:cancel-after-event", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_events" && tx.Error == nil {
			inserted = true
			cancel()
		}
	}))
	err := svc.appendEvent(ctx, rc.ConversationID, rc.RunID, evtTextDelta, map[string]string{"delta": "partial"})
	require.Error(t, err)
	require.True(t, inserted, "cancellation must occur after a successful event INSERT")
	require.NoError(t, svc.DB.Callback().Create().Remove("test:cancel-after-event"))
	var run store.AgentRun
	var events []store.AgentEvent
	require.NoError(t, svc.DB.First(&run, "id = ?", rc.RunID).Error)
	require.NoError(t, svc.DB.Where("run_id = ?", rc.RunID).Find(&events).Error)
	assert.EqualValues(t, 0, run.LastSequence)
	assert.Empty(t, events, "cancelled append must roll back both the event and its sequence")
	require.NoError(t, svc.finalizeRun(context.Background(), rc, "partial", nil, RunCancelled, "cancelled"))
	require.NoError(t, svc.DB.First(&run, "id = ?", rc.RunID).Error)
	assert.Equal(t, RunCancelled, run.Status)
	require.NoError(t, svc.DB.Where("run_id = ?", rc.RunID).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, evtRunFinished, events[0].EventType)
	assert.EqualValues(t, 1, events[0].Sequence)
	assert.EqualValues(t, 1, run.LastSequence)
}

func TestConcurrentEventAppendsHaveContiguousSequences(t *testing.T) {
	svc, rc := eventFixture(t)
	const count = 32
	start := make(chan struct{})
	errors := make(chan error, count)
	var writers sync.WaitGroup
	for i := range count {
		writers.Go(func() {
			<-start
			errors <- svc.appendEvent(context.Background(), rc.ConversationID, rc.RunID, evtTextDelta, map[string]int{"writer": i})
		})
	}
	close(start)
	writers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err, "concurrent append must not lose events to duplicate sequence or locking")
	}
	var events []store.AgentEvent
	require.NoError(t, svc.DB.Where("run_id = ?", rc.RunID).Order("sequence").Find(&events).Error)
	require.Len(t, events, count)
	for i, event := range events {
		assert.EqualValues(t, i+1, event.Sequence)
	}
	var run store.AgentRun
	require.NoError(t, svc.DB.First(&run, "id = ?", rc.RunID).Error)
	assert.EqualValues(t, count, run.LastSequence)
}
