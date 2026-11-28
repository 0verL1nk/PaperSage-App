package aiusage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func intPtr(v int) *int { return &v }

type seedSpec struct {
	day        string
	model      string
	feature    string
	status     string
	latency    int
	prompt     *int
	completion *int
	first      *int
	stream     bool
	errorCode  string
	user       *store.UUID
}

func seedCall(t *testing.T, db *gorm.DB, spec seedSpec) {
	t.Helper()
	created, err := time.ParseInLocation("2006-01-02 15:04", spec.day+" 12:00", time.UTC)
	require.NoError(t, err)
	row := &store.AIModelCall{
		ID: store.MustNewUUID(), UserID: spec.user,
		FeatureKind: spec.feature, Provider: "provider-a", ProviderSource: "user-settings",
		Model: spec.model, IsStream: spec.stream, Status: spec.status,
		LatencyMs: spec.latency, CreatedDay: spec.day, CreatedAt: created,
		PromptTokens: spec.prompt, CompletionTokens: spec.completion, FirstTokenMs: spec.first,
	}
	if spec.errorCode != "" {
		code := spec.errorCode
		row.ErrorCode = &code
	}
	require.NoError(t, db.Create(row).Error)
}

func seedFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedCall(t, db, seedSpec{day: "2026-11-27", model: "m1", feature: "chat", status: StatusSuccess, latency: 100, prompt: intPtr(10), completion: intPtr(5), first: intPtr(50), stream: true})
	seedCall(t, db, seedSpec{day: "2026-11-27", model: "m1", feature: "chat", status: StatusSuccess, latency: 200, prompt: intPtr(20), completion: intPtr(10), first: intPtr(70), stream: true})
	seedCall(t, db, seedSpec{day: "2026-11-27", model: "m2", feature: "chat", status: StatusSuccess, latency: 300})
	seedCall(t, db, seedSpec{day: "2026-11-28", model: "m1", feature: "cross_chapter_review", status: StatusSuccess, latency: 400, prompt: intPtr(40), completion: intPtr(20), first: intPtr(90), stream: true})
	seedCall(t, db, seedSpec{day: "2026-11-28", model: "m1", feature: "cross_chapter_review", status: StatusError, latency: 500, errorCode: CodeAuth})
	seedCall(t, db, seedSpec{day: "2026-11-28", model: "m1", feature: "chat", status: StatusTimeout, latency: 600})
	seedCall(t, db, seedSpec{day: "2026-11-28", model: "m2", feature: "chat", status: StatusCancelled, latency: 700})
}

func testFilter() Filter {
	return Filter{
		From: time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
	}
}

func TestAggregateTotalsAndGroups(t *testing.T) {
	db := newTestDB(t)
	seedFixture(t, db)
	svc := NewService(db, 180, "Asia/Shanghai")

	report, err := svc.Aggregate(context.Background(), testFilter())
	require.NoError(t, err)

	assert.Equal(t, int64(7), report.Totals.Calls)
	assert.Equal(t, int64(4), report.Totals.Success)
	assert.Equal(t, int64(1), report.Totals.Error)
	assert.Equal(t, int64(1), report.Totals.Timeout)
	assert.Equal(t, int64(1), report.Totals.Cancelled)
	assert.InDelta(t, 4.0/7.0, report.Totals.SuccessRate, 1e-9)
	assert.Equal(t, int64(70), report.Totals.PromptTokens)
	assert.Equal(t, int64(35), report.Totals.CompletionTokens)
	assert.Equal(t, int64(4), report.Totals.UsageMissingCalls)
	require.NotNil(t, report.Totals.LatencyP50Ms)
	assert.Equal(t, 200, *report.Totals.LatencyP50Ms, "4 个 success 中取第 2 小")
	require.NotNil(t, report.Totals.LatencyP95Ms)
	assert.Equal(t, 400, *report.Totals.LatencyP95Ms)
	require.NotNil(t, report.Totals.FirstTokenP50Ms)
	assert.Equal(t, 70, *report.Totals.FirstTokenP50Ms)

	require.Len(t, report.ByDay, 2)
	assert.Equal(t, "2026-11-27", report.ByDay[0].Key)
	assert.Equal(t, int64(3), report.ByDay[0].Calls)
	assert.Equal(t, int64(3), report.ByDay[0].Success)
	assert.Equal(t, "2026-11-28", report.ByDay[1].Key)
	assert.Equal(t, int64(4), report.ByDay[1].Calls)
	assert.Equal(t, int64(1), report.ByDay[1].Success)
	require.NotNil(t, report.ByDay[1].LatencyP50Ms)
	assert.Equal(t, 400, *report.ByDay[1].LatencyP50Ms)

	require.NotEmpty(t, report.ByModel)
	assert.Equal(t, "m1", report.ByModel[0].Key)
	assert.Equal(t, int64(5), report.ByModel[0].Calls)
	assert.Equal(t, int64(3), report.ByModel[0].Success)
	require.NotEmpty(t, report.ByFeature)
	assert.Equal(t, "chat", report.ByFeature[0].Key)
	assert.Equal(t, int64(5), report.ByFeature[0].Calls)
	require.Len(t, report.ByError, 1)
	assert.Equal(t, "auth", report.ByError[0].Key)
	assert.Equal(t, int64(1), report.ByError[0].Calls)
}

func TestAggregateFilters(t *testing.T) {
	db := newTestDB(t)
	user := store.UUID{UUID: uuid.MustParse("77777777-7777-4777-8777-777777777777")}
	seedFixture(t, db)
	seedCall(t, db, seedSpec{day: "2026-11-27", model: "m3", feature: "chat", status: StatusSuccess, latency: 111, user: &user})
	svc := NewService(db, 180, "Asia/Shanghai")

	f := testFilter()
	f.Model = "m2"
	report, err := svc.Aggregate(context.Background(), f)
	require.NoError(t, err)
	assert.Equal(t, int64(2), report.Totals.Calls)

	f = testFilter()
	f.FeatureKind = "cross_chapter_review"
	report, err = svc.Aggregate(context.Background(), f)
	require.NoError(t, err)
	assert.Equal(t, int64(2), report.Totals.Calls)

	f = testFilter()
	f.UserID = &user
	report, err = svc.Aggregate(context.Background(), f)
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Totals.Calls)
	assert.Equal(t, 111, *report.Totals.LatencyP50Ms)
}

func TestAggregateEmpty(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, 180, "Asia/Shanghai")
	report, err := svc.Aggregate(context.Background(), testFilter())
	require.NoError(t, err)
	assert.Equal(t, int64(0), report.Totals.Calls)
	assert.Zero(t, report.Totals.SuccessRate)
	assert.Nil(t, report.Totals.LatencyP50Ms)
	assert.Nil(t, report.Totals.LatencyP95Ms)
	assert.Nil(t, report.Totals.FirstTokenP50Ms)
	assert.Empty(t, report.ByDay)
	assert.Empty(t, report.ByModel)
}

func TestAggregateSingleSamplePercentiles(t *testing.T) {
	db := newTestDB(t)
	seedCall(t, db, seedSpec{day: "2026-11-27", model: "m1", feature: "chat", status: StatusSuccess, latency: 123})
	svc := NewService(db, 180, "Asia/Shanghai")
	report, err := svc.Aggregate(context.Background(), testFilter())
	require.NoError(t, err)
	require.NotNil(t, report.Totals.LatencyP50Ms)
	assert.Equal(t, 123, *report.Totals.LatencyP50Ms)
	require.NotNil(t, report.Totals.LatencyP95Ms)
	assert.Equal(t, 123, *report.Totals.LatencyP95Ms)
	assert.Nil(t, report.Totals.FirstTokenP50Ms, "非流式调用没有首字样本")
}

func TestExportRows(t *testing.T) {
	db := newTestDB(t)
	seedFixture(t, db)
	svc := NewService(db, 180, "Asia/Shanghai")
	rows, err := svc.ExportRows(context.Background(), testFilter())
	require.NoError(t, err)
	require.Len(t, rows, 5)
	for _, row := range rows {
		assert.Empty(t, row.Key)
		assert.NotEmpty(t, row.Day)
		assert.NotEmpty(t, row.Provider)
		assert.NotEmpty(t, row.Model)
		assert.NotEmpty(t, row.FeatureKind)
	}
	first := rows[0]
	assert.Equal(t, "2026-11-27", first.Day)
	assert.Equal(t, "m1", first.Model)
	assert.Equal(t, "chat", first.FeatureKind)
	assert.Equal(t, int64(2), first.Calls)
}
