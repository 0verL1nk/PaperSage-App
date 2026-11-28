// Package aiusage 记录模型调用计量（耗时 / Token / 状态）并提供聚合查询。
//
// 隐私红线：只存结构化计量与枚举错误分类，不存 prompt/回复正文、API Key、
// base_url、请求头或原始错误消息；token 缺失存 NULL，不补 0。
package aiusage

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // 生产镜像可能没有 zoneinfo；内嵌 tzdata 保证 Asia/Shanghai 可用
	"unicode"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	defaultRetentionDays = 180
	minRetentionDays     = 7
	maxRetentionDays     = 730
	cleanupInterval      = time.Hour
	persistTimeout       = 2 * time.Second

	maxFeatureRunes   = 32
	maxProviderRunes  = 64
	maxSourceRunes    = 20
	maxModelRunes     = 128
	maxErrorCodeRunes = 32
)

// Call 是一次模型调用的归属与结果，由 ChatModel 装饰器填写。
type Call struct {
	UserID           store.UUID
	ProjectID        store.UUID
	RunID            store.UUID
	FeatureKind      string
	Provider         string
	ProviderSource   string
	Model            string
	IsStream         bool
	Status           string
	ErrorCode        string
	HTTPStatus       *int16
	PromptTokens     *int
	CompletionTokens *int
	ReasoningTokens  *int
	LatencyMs        int
	FirstTokenMs     *int
}

// Recorder 由 agent 域消费；实现方负责落库且绝不影响 Run 状态。
type Recorder interface {
	Record(ctx context.Context, call Call)
}

// Service 持有 DB、保留期与时区配置。
type Service struct {
	DB            *gorm.DB
	RetentionDays int
	Location      *time.Location

	mu          sync.Mutex
	lastCleanup time.Time
	now         func() time.Time
}

// NewService 构造服务；非法时区回退 UTC 并告警，不阻断启动。
func NewService(db *gorm.DB, retentionDays int, timezone string) *Service {
	if retentionDays <= 0 {
		retentionDays = defaultRetentionDays
	}
	if retentionDays < minRetentionDays {
		retentionDays = minRetentionDays
	}
	if retentionDays > maxRetentionDays {
		retentionDays = maxRetentionDays
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		slog.Warn("aiusage: invalid AI_USAGE_TIMEZONE, falling back to UTC", "timezone", timezone, "err", err)
		loc = time.UTC
	}
	return &Service{DB: db, RetentionDays: retentionDays, Location: loc, now: time.Now}
}

// Record 落一条调用计量。失败只记日志，绝不影响 Run。
func (s *Service) Record(ctx context.Context, c Call) {
	if s == nil || s.DB == nil {
		return
	}
	now := s.now().UTC()
	row := &store.AIModelCall{
		ID:             store.MustNewUUID(),
		UserID:         optionalUUID(c.UserID),
		ProjectID:      optionalUUID(c.ProjectID),
		RunID:          optionalUUID(c.RunID),
		FeatureKind:    cleanText(c.FeatureKind, maxFeatureRunes, "chat"),
		Provider:       cleanText(c.Provider, maxProviderRunes, "unknown"),
		ProviderSource: cleanText(c.ProviderSource, maxSourceRunes, "unknown"),
		Model:          cleanText(c.Model, maxModelRunes, "unknown"),
		IsStream:       c.IsStream,
		Status:         normalizeStatus(c.Status),
		LatencyMs:      max(c.LatencyMs, 0),
		CreatedDay:     now.In(s.Location).Format("2006-01-02"),
		CreatedAt:      now,
	}
	if code := cleanText(c.ErrorCode, maxErrorCodeRunes, ""); code != "" {
		row.ErrorCode = &code
	}
	if c.HTTPStatus != nil && *c.HTTPStatus > 0 {
		v := *c.HTTPStatus
		row.HTTPStatus = &v
	}
	row.PromptTokens = nonNegative(c.PromptTokens)
	row.CompletionTokens = nonNegative(c.CompletionTokens)
	row.ReasoningTokens = nonNegative(c.ReasoningTokens)
	row.FirstTokenMs = nonNegative(c.FirstTokenMs)

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if err := s.DB.WithContext(writeCtx).Create(row).Error; err != nil {
		slog.Warn("aiusage: persist model call failed", "run_id", c.RunID.String(), "err", err)
		return
	}
	s.maybeCleanup(writeCtx)
}

// maybeCleanup 沿用请求路径惰性清理（与 compilediag 同模式），不引入定时器。
func (s *Service) maybeCleanup(ctx context.Context) {
	now := s.now()
	s.mu.Lock()
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < cleanupInterval {
		s.mu.Unlock()
		return
	}
	s.lastCleanup = now
	s.mu.Unlock()

	cutoff := now.AddDate(0, 0, -s.RetentionDays)
	res := s.DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&store.AIModelCall{})
	if res.Error != nil {
		slog.Warn("aiusage: retention cleanup failed", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("aiusage: retention cleanup removed expired calls", "rows", res.RowsAffected)
	}
}

func normalizeStatus(v string) string {
	switch v {
	case StatusSuccess, StatusError, StatusTimeout, StatusCancelled:
		return v
	}
	return StatusError
}

func optionalUUID(id store.UUID) *store.UUID {
	if id.UUID == [16]byte{} {
		return nil
	}
	v := id
	return &v
}

func nonNegative(v *int) *int {
	if v == nil {
		return nil
	}
	n := max(*v, 0)
	return &n
}

// cleanText 去除控制字符、截断到 max 个 rune；空值用 fallback。
func cleanText(v string, maxRunes int, fallback string) string {
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(v))
	runes := []rune(v)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	if len(runes) == 0 {
		return fallback
	}
	return string(runes)
}
