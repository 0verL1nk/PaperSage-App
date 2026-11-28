package aiusage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/aiusage.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&store.AIModelCall{}))
	return db
}

func fixedNow() time.Time { return time.Date(2026, 11, 27, 20, 30, 0, 0, time.UTC) }

func TestRecordPersistsTelemetry(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, 180, "Asia/Shanghai")
	svc.now = fixedNow

	user := store.UUID{UUID: uuid.MustParse("44444444-4444-4444-4444-444444444444")}
	project := store.UUID{UUID: uuid.MustParse("55555555-5555-4555-8555-555555555555")}
	run := store.UUID{UUID: uuid.MustParse("66666666-6666-4666-8666-666666666666")}
	prompt, completion, reasoning, first := 12, 34, 5, 220

	svc.Record(context.Background(), Call{
		UserID: user, ProjectID: project, RunID: run,
		FeatureKind: "cross_chapter_review", Provider: "DeepSeek", ProviderSource: "user-settings",
		Model: "deepseek-chat", IsStream: true, Status: StatusSuccess,
		PromptTokens: &prompt, CompletionTokens: &completion, ReasoningTokens: &reasoning,
		LatencyMs: 1234, FirstTokenMs: &first,
	})

	var row store.AIModelCall
	require.NoError(t, db.First(&row).Error)
	require.NotNil(t, row.UserID)
	assert.Equal(t, user.UUID, row.UserID.UUID)
	require.NotNil(t, row.ProjectID)
	assert.Equal(t, project.UUID, row.ProjectID.UUID)
	require.NotNil(t, row.RunID)
	assert.Equal(t, run.UUID, row.RunID.UUID)
	assert.Equal(t, "cross_chapter_review", row.FeatureKind)
	assert.Equal(t, "DeepSeek", row.Provider)
	assert.Equal(t, "user-settings", row.ProviderSource)
	assert.Equal(t, "deepseek-chat", row.Model)
	assert.True(t, row.IsStream)
	assert.Equal(t, StatusSuccess, row.Status)
	require.NotNil(t, row.PromptTokens)
	assert.Equal(t, 12, *row.PromptTokens)
	require.NotNil(t, row.CompletionTokens)
	assert.Equal(t, 34, *row.CompletionTokens)
	require.NotNil(t, row.ReasoningTokens)
	assert.Equal(t, 5, *row.ReasoningTokens)
	assert.Equal(t, 1234, row.LatencyMs)
	require.NotNil(t, row.FirstTokenMs)
	assert.Equal(t, 220, *row.FirstTokenMs)
	// UTC 2026-11-27 20:30 = 北京时间 2026-11-28 04:30
	assert.Equal(t, "2026-11-28", row.CreatedDay)
	assert.True(t, row.CreatedAt.Equal(fixedNow()))
}

func TestRecordUsageMissingStaysNull(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, 180, "Asia/Shanghai")
	svc.now = fixedNow

	svc.Record(context.Background(), Call{
		FeatureKind: "chat", Provider: "p", ProviderSource: "server-env", Model: "m",
		Status: StatusError, ErrorCode: CodeNetwork, LatencyMs: 10,
	})

	var row store.AIModelCall
	require.NoError(t, db.First(&row).Error)
	assert.Nil(t, row.PromptTokens, "provider 未返回 usage 时必须存 NULL，不能补 0")
	assert.Nil(t, row.CompletionTokens)
	assert.Nil(t, row.ReasoningTokens)
	assert.Nil(t, row.FirstTokenMs)
	assert.Nil(t, row.UserID)
	assert.Nil(t, row.ProjectID)
	assert.Nil(t, row.RunID)
	require.NotNil(t, row.ErrorCode)
	assert.Equal(t, CodeNetwork, *row.ErrorCode)
}

func TestRecordSanitizesTextFields(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, 180, "Asia/Shanghai")
	svc.now = fixedNow

	svc.Record(context.Background(), Call{
		FeatureKind: "", Provider: "Pro\nvider\x00", ProviderSource: "unknown-source",
		Model: strings.Repeat("m", 200), Status: "not-a-status", LatencyMs: -5,
	})

	var row store.AIModelCall
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, "chat", row.FeatureKind)
	assert.Equal(t, "Provider", row.Provider)
	assert.Len(t, []rune(row.Model), maxModelRunes)
	assert.Equal(t, StatusError, row.Status)
	assert.Equal(t, 0, row.LatencyMs)
}

func TestRecordCleansExpiredRows(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, 180, "Asia/Shanghai")
	svc.now = fixedNow

	expired := &store.AIModelCall{
		ID: store.MustNewUUID(), FeatureKind: "chat", Provider: "p", ProviderSource: "server-env",
		Model: "m", Status: StatusSuccess, CreatedDay: "2026-05-01",
		CreatedAt: fixedNow().AddDate(0, 0, -200),
	}
	require.NoError(t, db.Create(expired).Error)

	svc.Record(context.Background(), Call{FeatureKind: "chat", Provider: "p", ProviderSource: "server-env", Model: "m", Status: StatusSuccess})

	var count int64
	require.NoError(t, db.Model(&store.AIModelCall{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "超过保留期的行应被硬删")
}

func TestNewServiceClampsRetentionAndFallsBackTimezone(t *testing.T) {
	svc := NewService(nil, 1, "Not/AZone")
	assert.Equal(t, minRetentionDays, svc.RetentionDays)
	assert.Equal(t, time.UTC, svc.Location)

	svc = NewService(nil, 5000, "Asia/Shanghai")
	assert.Equal(t, maxRetentionDays, svc.RetentionDays)
	assert.Equal(t, "Asia/Shanghai", svc.Location.String())
}

func TestNilServiceRecordIsNoop(t *testing.T) {
	var svc *Service
	assert.NotPanics(t, func() { svc.Record(context.Background(), Call{}) })
	assert.NotPanics(t, func() { NewService(nil, 180, "Asia/Shanghai").Record(context.Background(), Call{}) })
}
