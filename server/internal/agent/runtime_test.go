package agent

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
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

var errCommitAckLost = errors.New("test: commit acknowledgement lost")

type ackLossPool struct {
	*sql.DB
	armed         atomic.Bool
	committed     chan struct{}
	release       chan struct{}
	cancelRequest context.CancelFunc
	rollback      bool
}

func (p *ackLossPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &ackLossTx{Tx: tx, pool: p, inject: p.armed.CompareAndSwap(true, false)}, nil
}

type ackLossTx struct {
	*sql.Tx
	pool   *ackLossPool
	inject bool
}

func (tx *ackLossTx) Commit() error {
	if !tx.inject {
		return tx.Tx.Commit()
	}
	if tx.pool.rollback {
		if err := tx.Rollback(); err != nil {
			return err
		}
		return errCommitAckLost
	}
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	if tx.pool.cancelRequest != nil {
		tx.pool.cancelRequest()
	}
	if tx.pool.committed != nil {
		close(tx.pool.committed)
		<-tx.pool.release
	}
	return errCommitAckLost
}

func TestRootRunCommitAckLoss(t *testing.T) {
	for _, key := range []string{"same-key", ""} {
		t.Run(map[bool]string{true: "keyed", false: "no-key"}[key != ""], func(t *testing.T) {
			svc, rc := eventFixture(t)
			var calls atomic.Int32
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "scripted failure", http.StatusBadRequest)
			}))
			t.Cleanup(fake.Close)
			t.Cleanup(func() { _ = svc.Close(context.Background()) })
			svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: fake.URL, AgentModel: "fake"}
			var conv store.AgentConversation
			require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
			sqlDB, err := svc.DB.DB()
			require.NoError(t, err)
			pool := &ackLossPool{DB: sqlDB, committed: make(chan struct{}), release: make(chan struct{})}
			wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			svc.DB = wrapped
			pool.armed.Store(true)
			type outcome struct {
				result *rootRunResult
				err    error
			}
			first := make(chan outcome, 1)
			go func() {
				version := int64(0)
				result, err := svc.createRootRun(context.Background(), conv, conv.UserID, key, "question", &version)
				first <- outcome{result, err}
			}()
			select {
			case <-pool.committed:
			case <-time.After(5 * time.Second):
				t.Fatal("transaction did not commit")
			}
			var replay *rootRunResult
			if key != "" {
				version := int64(0)
				replay, err = svc.createRootRun(context.Background(), conv, conv.UserID, key, "question", &version)
				require.NoError(t, err)
				assert.False(t, replay.created, "a competing replay must not start this Run")
				assert.Zero(t, calls.Load())
			}
			close(pool.release)
			got := <-first
			require.NoError(t, got.err)
			require.NotNil(t, got.result)
			assert.True(t, got.result.created, "the committing request must start its own Run")
			require.NotNil(t, got.result.model)
			if replay != nil {
				assert.Equal(t, replay.run.ID, got.result.run.ID)
			}
			require.NoError(t, svc.StartRun(runContext{RunID: got.result.run.ID, ConversationID: conv.ID, ProjectID: conv.ProjectID, UserID: conv.UserID, Content: "question", Model: got.result.model}))
			require.Eventually(t, func() bool { return calls.Load() == 1 }, 3*time.Second, 10*time.Millisecond)
			var turns, roots, started int64
			require.NoError(t, svc.DB.Model(&store.AgentTurn{}).Count(&turns).Error)
			require.NoError(t, svc.DB.Model(&store.AgentRun{}).Where("turn_id IS NOT NULL").Count(&roots).Error)
			require.NoError(t, svc.DB.Model(&store.AgentEvent{}).Where("run_id = ? AND event_type = ?", got.result.run.ID, evtRunStarted).Count(&started).Error)
			assert.EqualValues(t, 1, turns)
			assert.EqualValues(t, 1, roots)
			assert.EqualValues(t, 1, started)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestRootRunUnconfirmedCommitDoesNotStart(t *testing.T) {
	for _, key := range []string{"same-key", ""} {
		t.Run(map[bool]string{true: "keyed", false: "no-key"}[key != ""], func(t *testing.T) {
			svc, rc := eventFixture(t)
			svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: "http://127.0.0.1:1", AgentModel: "fake"}
			var conv store.AgentConversation
			require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
			sqlDB, err := svc.DB.DB()
			require.NoError(t, err)
			pool := &ackLossPool{DB: sqlDB, rollback: true}
			wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			svc.DB = wrapped
			pool.armed.Store(true)
			version := int64(0)
			result, err := svc.createRootRun(context.Background(), conv, conv.UserID, key, "question", &version)
			assert.ErrorIs(t, err, errCommitUnknown)
			assert.Nil(t, result)
			var turns, roots int64
			require.NoError(t, svc.DB.Model(&store.AgentTurn{}).Count(&turns).Error)
			require.NoError(t, svc.DB.Model(&store.AgentRun{}).Where("turn_id IS NOT NULL").Count(&roots).Error)
			var updated store.AgentConversation
			require.NoError(t, svc.DB.First(&updated, "id = ?", conv.ID).Error)
			assert.Zero(t, turns)
			assert.Zero(t, roots)
			assert.Zero(t, updated.Version)
		})
	}
}

func TestRootRunCommitAckLossAfterRequestCancellation(t *testing.T) {
	svc, rc := eventFixture(t)
	svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: "http://127.0.0.1:1", AgentModel: "fake"}
	var conv store.AgentConversation
	require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
	sqlDB, err := svc.DB.DB()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := &ackLossPool{DB: sqlDB, cancelRequest: cancel}
	wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	svc.DB = wrapped
	pool.armed.Store(true)
	version := int64(0)
	result, err := svc.createRootRun(ctx, conv, conv.UserID, "key", "question", &version)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.created)
	assert.NotNil(t, result.model)
}

func TestRootRunCommitAckLossWhenReconciliationUnavailable(t *testing.T) {
	svc, rc := eventFixture(t)
	svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: "http://127.0.0.1:1", AgentModel: "fake"}
	var conv store.AgentConversation
	require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
	sqlDB, err := svc.DB.DB()
	require.NoError(t, err)
	var failRead atomic.Bool
	pool := &ackLossPool{DB: sqlDB, cancelRequest: func() { failRead.Store(true) }}
	wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, wrapped.Callback().Query().Before("gorm:query").Register("test:fail-reconciliation", func(tx *gorm.DB) {
		if failRead.Load() && tx.Statement.Table == "agent_conversations" {
			_ = tx.AddError(errors.New("test: reconciliation database unavailable"))
		}
	}))
	svc.DB = wrapped
	pool.armed.Store(true)
	version := int64(0)
	result, err := svc.createRootRun(context.Background(), conv, conv.UserID, "key", "question", &version)
	assert.ErrorIs(t, err, errCommitUnknown)
	assert.Nil(t, result)
	var runs int64
	require.NoError(t, svc.DB.Model(&store.AgentRun{}).Where("turn_id IS NOT NULL").Count(&runs).Error)
	assert.EqualValues(t, 1, runs, "an unconfirmed committed Run must not be replaced or started")
}

func TestRootRunCommitAckLossDuringShutdown(t *testing.T) {
	svc, rc := eventFixture(t)
	svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: "http://127.0.0.1:1", AgentModel: "fake"}
	var conv store.AgentConversation
	require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
	sqlDB, err := svc.DB.DB()
	require.NoError(t, err)
	pool := &ackLossPool{DB: sqlDB, committed: make(chan struct{}), release: make(chan struct{})}
	wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	svc.DB = wrapped
	pool.armed.Store(true)
	ctx, releaseLease, accepted := svc.acceptRun(context.Background())
	require.True(t, accepted)
	type outcome struct {
		result *rootRunResult
		err    error
	}
	first := make(chan outcome, 1)
	go func() {
		version := int64(0)
		result, err := svc.createRootRun(ctx, conv, conv.UserID, "key", "question", &version)
		first <- outcome{result, err}
	}()
	select {
	case <-pool.committed:
	case <-time.After(5 * time.Second):
		t.Fatal("transaction did not commit")
	}
	closed := make(chan error, 1)
	go func() { closed <- svc.Close(context.Background()) }()
	require.Eventually(t, func() bool { svc.cancelMu.Lock(); defer svc.cancelMu.Unlock(); return svc.closing }, time.Second, time.Millisecond)
	close(pool.release)
	got := <-first
	require.NoError(t, got.err)
	require.True(t, got.result.created)
	err = svc.StartRun(runContext{RunID: got.result.run.ID, ConversationID: conv.ID, ProjectID: conv.ProjectID, UserID: conv.UserID, Content: "question", Model: got.result.model, finalizeDeadline: got.result.finalizeDeadline})
	assert.ErrorIs(t, err, errServiceClosing)
	releaseLease()
	require.NoError(t, <-closed)
	var run store.AgentRun
	require.NoError(t, svc.DB.First(&run, "id = ?", got.result.run.ID).Error)
	assert.Equal(t, RunCancelled, run.Status)
}

func TestRootRunCommitAckLossAfterShutdownDeadline(t *testing.T) {
	svc, rc := eventFixture(t)
	var modelCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modelCalls.Add(1)
		http.Error(w, "unexpected model call", http.StatusBadRequest)
	}))
	t.Cleanup(fake.Close)
	svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: fake.URL, AgentModel: "fake"}
	var conv store.AgentConversation
	require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
	sqlDB, err := svc.DB.DB()
	require.NoError(t, err)
	pool := &ackLossPool{DB: sqlDB, committed: make(chan struct{}), release: make(chan struct{})}
	wrapped, err := gorm.Open(&sqlite.Dialector{Conn: pool}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	svc.DB = wrapped
	var tracking atomic.Bool
	reconcileDeadline, finalizeDeadline := make(chan time.Time, 1), make(chan time.Time, 1)
	require.NoError(t, wrapped.Callback().Query().Before("gorm:query").Register("test:reconcile-deadline", func(tx *gorm.DB) {
		if tracking.Load() && tx.Statement.Table == "agent_conversations" {
			deadline, _ := tx.Statement.Context.Deadline()
			select {
			case reconcileDeadline <- deadline:
			default:
			}
		}
	}))
	require.NoError(t, wrapped.Callback().Update().Before("gorm:update").Register("test:finalize-deadline", func(tx *gorm.DB) {
		if tracking.Load() && tx.Statement.Table == "agent_runs" {
			deadline, _ := tx.Statement.Context.Deadline()
			select {
			case finalizeDeadline <- deadline:
			default:
			}
		}
	}))
	pool.armed.Store(true)
	ctx, releaseLease, accepted := svc.acceptRun(context.Background())
	require.True(t, accepted)
	result := make(chan error, 1)
	go func() {
		defer releaseLease()
		version := int64(0)
		created, createErr := svc.createRootRun(ctx, conv, conv.UserID, "key", "question", &version)
		if createErr == nil && created.created {
			createErr = svc.StartRun(runContext{RunID: created.run.ID, ConversationID: conv.ID, ProjectID: conv.ProjectID, UserID: conv.UserID, Content: "question", Model: created.model, finalizeDeadline: created.finalizeDeadline})
		}
		result <- createErr
	}()
	select {
	case <-pool.committed:
	case <-time.After(5 * time.Second):
		close(pool.release)
		t.Fatal("transaction did not commit")
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- svc.Close(closeCtx) }()
	select {
	case <-svc.runCtx.Done():
	case <-time.After(5 * time.Second):
		close(pool.release)
		t.Fatal("shutdown did not reach its deadline")
	}
	select {
	case err := <-closed:
		close(pool.release)
		t.Fatalf("Close returned before the admitted request released its lease: %v", err)
	default:
	}
	require.NoError(t, sqlDB.PingContext(context.Background()), "DB must stay available while Close waits for the lease")
	tracking.Store(true)
	close(pool.release)
	assert.ErrorIs(t, <-result, errServiceClosing)
	assert.ErrorIs(t, <-closed, context.DeadlineExceeded)
	var lookupDue, finishDue time.Time
	select {
	case lookupDue = <-reconcileDeadline:
	default:
		t.Error("missing reconciliation deadline")
	}
	select {
	case finishDue = <-finalizeDeadline:
	default:
		t.Error("missing finalization deadline")
	}
	assert.False(t, lookupDue.IsZero())
	assert.Equal(t, lookupDue, finishDue, "reconciliation and finalization must share one budget")
	assert.Zero(t, modelCalls.Load(), "shutdown must not start the model")
	tracking.Store(false)
	var run store.AgentRun
	require.NoError(t, svc.DB.Where("turn_id IS NOT NULL").First(&run).Error)
	assert.Equal(t, RunCancelled, run.Status, "a confirmed committed Run must be finalized before DB closure")
}

func TestClosingRunDoesNotResetExpiredFinalizationBudget(t *testing.T) {
	svc, rc := eventFixture(t)
	require.NoError(t, svc.Close(context.Background()))
	rc.finalizeDeadline = time.Now().Add(-time.Second)
	err := svc.StartRun(rc)
	assert.ErrorIs(t, err, errServiceClosing)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	var run store.AgentRun
	require.NoError(t, svc.DB.First(&run, "id = ?", rc.RunID).Error)
	assert.Equal(t, RunRunning, run.Status, "an expired shared budget must not be silently reset")
}

func TestUnknownCommitErrorIsExplicitHTTPFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	(&Service{}).writeRootRunError(ctx, store.UUID{}, store.UUID{}, errCommitUnknown)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "无法确认")
	assert.Contains(t, rec.Body.String(), "勿重新发送")
}

func TestRootRunConcurrentSQLiteWriters(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "same key", false: "different keys"}[sameKey], func(t *testing.T) {
			svc, rc := eventFixture(t)
			svc.Cfg = &config.Settings{AgentAPIKey: "fake", AgentBaseURL: "http://127.0.0.1:1", AgentModel: "fake"}
			var conv store.AgentConversation
			require.NoError(t, svc.DB.First(&conv, "id = ?", rc.ConversationID).Error)
			entered, release := make(chan struct{}, 2), make(chan struct{})
			require.NoError(t, svc.DB.Callback().Update().Before("gorm:update").Register("test:race-root", func(tx *gorm.DB) {
				if tx.Statement.Table == "agent_conversations" {
					entered <- struct{}{}
					<-release
				}
			}))
			keys := []string{"one", "one"}
			if !sameKey {
				keys[1] = "two"
			}
			type outcome struct {
				result *rootRunResult
				err    error
			}
			results := make(chan outcome, 2)
			var wg sync.WaitGroup
			for _, key := range keys {
				wg.Add(1)
				go func(key string) {
					defer wg.Done()
					version := int64(0)
					result, err := svc.createRootRun(context.Background(), conv, conv.UserID, key, "question", &version)
					results <- outcome{result, err}
				}(key)
			}
			for range keys {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					close(release)
					t.Fatal("writers did not both reach CAS")
				}
			}
			close(release)
			wg.Wait()
			close(results)
			require.NoError(t, svc.DB.Callback().Update().Remove("test:race-root"))
			successes, creators := 0, 0
			for result := range results {
				if result.err == nil {
					successes++
					if result.result.created {
						creators++
					}
				} else {
					assert.True(t, result.err == errThreadConflict || strings.Contains(result.err.Error(), "locked"), result.err)
				}
			}
			assert.GreaterOrEqual(t, successes, 1)
			assert.Equal(t, 1, creators, "only the committing request may start the Run")
			var turns, users, roots int64
			require.NoError(t, svc.DB.Model(&store.AgentTurn{}).Count(&turns).Error)
			require.NoError(t, svc.DB.Model(&store.AgentMessage{}).Where("role = ?", "user").Count(&users).Error)
			require.NoError(t, svc.DB.Model(&store.AgentRun{}).Where("turn_id IS NOT NULL").Count(&roots).Error)
			assert.EqualValues(t, 1, turns)
			assert.EqualValues(t, 1, users)
			assert.EqualValues(t, 1, roots)
			var updated store.AgentConversation
			require.NoError(t, svc.DB.First(&updated, "id = ?", conv.ID).Error)
			assert.EqualValues(t, 1, updated.Version)
			if sameKey {
				version := int64(0)
				replay, err := svc.createRootRun(context.Background(), conv, conv.UserID, "one", "question", &version)
				require.NoError(t, err)
				assert.False(t, replay.created)
			}
		})
	}
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
	require.NoError(t, svc.finalizeRun(context.Background(), rc, "partial", RunCancelled, "cancelled"))
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
