package review

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 重启收敛或用户取消已经把任务写成终态后，本进程的 finishRun 不得覆盖：
// 两个执行器互相覆盖会把仍在跑的单元写成「未在时限内完成」，任务状态也会来回跳。
func TestFinishRunDoesNotOverwriteTerminalRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/review-guard.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	runID := store.MustNewUUID()
	run := store.ReviewRun{
		ID: runID, ProjectID: store.MustNewUUID(), UserID: store.MustNewUUID(),
		ConversationID: store.MustNewUUID(), ClientRequestID: "guard-1",
		Status: store.ReviewRunFailed, ScopeJSON: datatypes.JSON(`{}`),
		ErrorText: "服务重启，检查已中断，请重新运行",
	}
	require.NoError(t, db.Create(&run).Error)
	unit := store.ReviewUnit{
		ID: store.MustNewUUID(), ReviewRunID: runID, Seq: 1, Kind: "chapter", Label: "绪论",
		FilePathsJSON: datatypes.JSON(`[]`), SectionIDsJSON: datatypes.JSON(`[]`),
		DimensionsJSON: datatypes.JSON(`["goal_conclusion"]`), ContextJSON: datatypes.JSON(`{}`),
		InputFingerprint: "fp", Status: store.ReviewUnitRunning,
		FindingsJSON: datatypes.JSON(`[]`), DimensionResultsJSON: datatypes.JSON(`[]`),
		SourcesReadJSON: datatypes.JSON(`[]`),
	}
	require.NoError(t, db.Create(&unit).Error)

	svc := &Service{DB: db}
	exec := &runExecutor{
		svc: svc, runID: runID,
		current: map[string]store.UUID{}, results: map[string]*unitResult{},
	}
	svc.finishRun(context.Background(), exec, &run, nil)

	var gotRun store.ReviewRun
	require.NoError(t, db.First(&gotRun, "id = ?", runID).Error)
	require.Equal(t, store.ReviewRunFailed, gotRun.Status)
	require.Equal(t, "服务重启，检查已中断，请重新运行", gotRun.ErrorText)

	var gotUnit store.ReviewUnit
	require.NoError(t, db.First(&gotUnit, "id = ?", unit.ID).Error)
	require.Equal(t, store.ReviewUnitRunning, gotUnit.Status,
		"a terminal run must not rewrite its units")
}
