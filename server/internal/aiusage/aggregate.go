package aiusage

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const keySep = "\x1f"

// Filter 是聚合查询的窗口与筛选条件。[From, To) 为 UTC 绝对时间。
type Filter struct {
	UserID      *store.UUID
	From        time.Time
	To          time.Time
	Model       string
	FeatureKind string
}

// Bucket 一个分组的聚合指标。Token 与分位数为 nil 表示没有可统计的样本。
type Bucket struct {
	Key               string  `json:"key,omitempty"`
	Calls             int64   `json:"calls"`
	Success           int64   `json:"success"`
	Error             int64   `json:"error"`
	Timeout           int64   `json:"timeout"`
	Cancelled         int64   `json:"cancelled"`
	SuccessRate       float64 `json:"success_rate"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	UsageMissingCalls int64   `json:"usage_missing_calls"`
	LatencyP50Ms      *int    `json:"latency_p50_ms"`
	LatencyP95Ms      *int    `json:"latency_p95_ms"`
	FirstTokenP50Ms   *int    `json:"first_token_p50_ms"`
}

// Report 管理端与个人端共用的响应：总量 + 按日 / 模型 / 功能 / 错误码。
type Report struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Timezone  string   `json:"timezone"`
	Totals    Bucket   `json:"totals"`
	ByDay     []Bucket `json:"by_day"`
	ByModel   []Bucket `json:"by_model"`
	ByFeature []Bucket `json:"by_feature"`
	ByError   []Bucket `json:"by_error"`
}

// ExportRow 是导出用的聚合行：日 × provider × model × feature，不含用户维度。
type ExportRow struct {
	Day         string `json:"day"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	FeatureKind string `json:"feature_kind"`
	Bucket
}

type groupRow struct {
	keys            []string
	bucket          Bucket
	firstTokenCalls int64
}

// Aggregate 汇总窗口内的调用。分位数只统计 success 调用；首字分位仅统计
// success 且流式、provider 返回过首字时间的调用。
func (s *Service) Aggregate(ctx context.Context, f Filter) (*Report, error) {
	totalsRows, err := s.groupRows(ctx, f, nil, nil)
	if err != nil {
		return nil, err
	}
	totals := Bucket{}
	if len(totalsRows) == 1 {
		totals = totalsRows[0].bucket
	}

	byDay, err := s.groupRows(ctx, f, []string{"created_day"}, nil)
	if err != nil {
		return nil, err
	}
	byModel, err := s.groupRows(ctx, f, []string{"model"}, nil)
	if err != nil {
		return nil, err
	}
	byFeature, err := s.groupRows(ctx, f, []string{"feature_kind"}, nil)
	if err != nil {
		return nil, err
	}
	byError, err := s.groupRows(ctx, f, []string{"error_code"}, []string{"status <> 'success'", "error_code IS NOT NULL"})
	if err != nil {
		return nil, err
	}

	sort.Slice(byModel, func(i, j int) bool {
		if byModel[i].bucket.Calls != byModel[j].bucket.Calls {
			return byModel[i].bucket.Calls > byModel[j].bucket.Calls
		}
		return byModel[i].keys[0] < byModel[j].keys[0]
	})
	sort.Slice(byFeature, func(i, j int) bool {
		if byFeature[i].bucket.Calls != byFeature[j].bucket.Calls {
			return byFeature[i].bucket.Calls > byFeature[j].bucket.Calls
		}
		return byFeature[i].keys[0] < byFeature[j].keys[0]
	})
	sort.Slice(byError, func(i, j int) bool {
		if byError[i].bucket.Calls != byError[j].bucket.Calls {
			return byError[i].bucket.Calls > byError[j].bucket.Calls
		}
		return byError[i].keys[0] < byError[j].keys[0]
	})

	return &Report{
		From:      f.From.UTC().Format(time.RFC3339),
		To:        f.To.UTC().Format(time.RFC3339),
		Timezone:  s.Location.String(),
		Totals:    totals,
		ByDay:     buckets(byDay),
		ByModel:   buckets(byModel),
		ByFeature: buckets(byFeature),
		ByError:   buckets(byError),
	}, nil
}

// ExportRows 按日 × provider × model × feature 汇总；不含 user_id 与逐次明细。
func (s *Service) ExportRows(ctx context.Context, f Filter) ([]ExportRow, error) {
	rows, err := s.groupRows(ctx, f, []string{"created_day", "provider", "model", "feature_kind"}, nil)
	if err != nil {
		return nil, err
	}
	out := make([]ExportRow, 0, len(rows))
	for _, r := range rows {
		if len(r.keys) != 4 {
			continue
		}
		b := r.bucket
		b.Key = ""
		out = append(out, ExportRow{
			Day: r.keys[0], Provider: r.keys[1], Model: r.keys[2], FeatureKind: r.keys[3],
			Bucket: b,
		})
	}
	return out, nil
}

func buckets(rows []groupRow) []Bucket {
	out := make([]Bucket, 0, len(rows))
	for _, r := range rows {
		b := r.bucket
		if len(r.keys) == 1 {
			b.Key = r.keys[0]
		}
		out = append(out, b)
	}
	return out
}

func (s *Service) filtered(ctx context.Context, f Filter) *gorm.DB {
	q := s.DB.WithContext(ctx).Table("ai_model_calls")
	if f.UserID != nil {
		q = q.Where("user_id = ?", *f.UserID)
	}
	if !f.From.IsZero() {
		q = q.Where("created_at >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("created_at < ?", f.To)
	}
	if f.Model != "" {
		q = q.Where("model = ?", f.Model)
	}
	if f.FeatureKind != "" {
		q = q.Where("feature_kind = ?", f.FeatureKind)
	}
	return q
}

// groupRows 两遍查询：先 GROUP BY 出计数与 token 和，再按分组 + 延迟排序
// 流式扫描取精确分位值（ceil(p·n)，1-indexed）。内存 O(分组数)。
func (s *Service) groupRows(ctx context.Context, f Filter, cols []string, extra []string) ([]groupRow, error) {
	rows, err := s.countRows(ctx, f, cols, extra)
	if err != nil {
		return nil, err
	}
	successCounts := make(map[string]int64, len(rows))
	firstTokenCounts := make(map[string]int64, len(rows))
	for _, r := range rows {
		k := strings.Join(r.keys, keySep)
		successCounts[k] = r.bucket.Success
		firstTokenCounts[k] = r.firstTokenCalls
	}

	latency, err := s.percentileValues(ctx, f, cols, extra, "latency_ms", successCounts, true, []float64{0.5, 0.95})
	if err != nil {
		return nil, err
	}
	firstToken, err := s.percentileValues(ctx, f, cols, extra, "first_token_ms", firstTokenCounts, true, []float64{0.5})
	if err != nil {
		return nil, err
	}

	for i := range rows {
		k := strings.Join(rows[i].keys, keySep)
		if vals := latency[k]; len(vals) == 2 {
			rows[i].bucket.LatencyP50Ms = vals[0]
			rows[i].bucket.LatencyP95Ms = vals[1]
		}
		if vals := firstToken[k]; len(vals) == 1 {
			rows[i].bucket.FirstTokenP50Ms = vals[0]
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.Join(rows[i].keys, keySep) < strings.Join(rows[j].keys, keySep)
	})
	return rows, nil
}

func (s *Service) countRows(ctx context.Context, f Filter, cols []string, extra []string) ([]groupRow, error) {
	q := s.filtered(ctx, f)
	for _, w := range extra {
		q = q.Where(w)
	}
	selects := []string{
		"COUNT(*) AS calls",
		"COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success",
		"COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) AS err_calls",
		"COALESCE(SUM(CASE WHEN status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_calls",
		"COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) AS cancelled_calls",
		"COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens",
		"COALESCE(SUM(completion_tokens), 0) AS completion_tokens",
		"COALESCE(SUM(CASE WHEN prompt_tokens IS NULL THEN 1 ELSE 0 END), 0) AS usage_missing",
		"COALESCE(SUM(CASE WHEN status = 'success' AND is_stream = " + s.trueLiteral() + " AND first_token_ms IS NOT NULL THEN 1 ELSE 0 END), 0) AS first_token_calls",
	}
	if len(cols) > 0 {
		selects = append(append([]string{}, cols...), selects...)
		q = q.Group(strings.Join(cols, ", "))
	}
	rows, err := q.Select(strings.Join(selects, ", ")).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupRow, 0)
	for rows.Next() {
		keys := make([]string, len(cols))
		dest := make([]any, 0, len(cols)+9)
		for i := range keys {
			dest = append(dest, &keys[i])
		}
		var calls, success, errCalls, timeout, cancelled, promptTokens, completionTokens, usageMissing, firstTokenCalls int64
		dest = append(dest, &calls, &success, &errCalls, &timeout, &cancelled,
			&promptTokens, &completionTokens, &usageMissing, &firstTokenCalls)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		b := Bucket{
			Calls: calls, Success: success, Error: errCalls,
			Timeout: timeout, Cancelled: cancelled,
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			UsageMissingCalls: usageMissing,
		}
		if calls > 0 {
			b.SuccessRate = float64(success) / float64(calls)
		}
		out = append(out, groupRow{keys: keys, bucket: b, firstTokenCalls: firstTokenCalls})
	}
	return out, rows.Err()
}

// percentileValues 只统计匹配行；counts 给出每个分组的目标样本数，用于算 rank。
// col 必须是白名单列（latency_ms / first_token_ms），不会拼接用户输入。
func (s *Service) percentileValues(ctx context.Context, f Filter, cols []string, extra []string,
	col string, counts map[string]int64, successOnly bool, ps []float64) (map[string][]*int, error) {
	q := s.filtered(ctx, f)
	for _, w := range extra {
		q = q.Where(w)
	}
	if successOnly {
		q = q.Where("status = ?", StatusSuccess)
	}
	if col == "first_token_ms" {
		q = q.Where("first_token_ms IS NOT NULL AND is_stream = ?", true)
	}
	order := col
	selects := col
	if len(cols) > 0 {
		order = strings.Join(cols, ", ") + ", " + col
		selects = strings.Join(cols, ", ") + ", " + col
	}
	rows, err := q.Select(selects).Order(order).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]int, len(counts))
	out := make(map[string][]*int, len(counts))
	for rows.Next() {
		keys := make([]string, len(cols))
		dest := make([]any, 0, len(cols)+1)
		for i := range keys {
			dest = append(dest, &keys[i])
		}
		var value int
		dest = append(dest, &value)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		k := strings.Join(keys, keySep)
		idx := seen[k]
		seen[k] = idx + 1
		for i, p := range ps {
			if int64(idx+1) != percentileRank(p, counts[k]) {
				continue
			}
			vals := out[k]
			if vals == nil {
				vals = make([]*int, len(ps))
				out[k] = vals
			}
			v := value
			vals[i] = &v
		}
	}
	return out, rows.Err()
}

func percentileRank(p float64, n int64) int64 {
	if n <= 0 {
		return 0
	}
	rank := int64(math.Ceil(p * float64(n)))
	if rank < 1 {
		return 1
	}
	if rank > n {
		return n
	}
	return rank
}

// trueLiteral 返回当前方言的布尔真值字面量（SQLite/MySQL 用 1，PostgreSQL 用 true）。
func (s *Service) trueLiteral() string {
	if s.DB != nil && s.DB.Dialector != nil && s.DB.Dialector.Name() == "postgres" {
		return "true"
	}
	return "1"
}
