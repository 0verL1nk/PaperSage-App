package agent

import (
	"context"
	"testing"
	"time"

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
		StartedAt: &old,
	}
	fresh := time.Now().UTC().Add(time.Hour)
	live := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(), AgentKey: AgentKeyMain,
		Status: RunRunning, ModelProvider: "p", ModelName: "m", PromptVersion: "v",
		StartedAt: &fresh,
	}
	require.NoError(t, db.Create(&orphan).Error)
	require.NoError(t, db.Create(&live).Error)

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

	got = store.AgentRun{}
	require.NoError(t, db.First(&got, "id = ?", live.ID).Error)
	assert.Equal(t, RunRunning, got.Status, "本进程启动后创建的 Run 不得被误杀")
}
