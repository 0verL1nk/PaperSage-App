package aiusage

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	adminDefaultDays    = 7
	personalDefaultDays = 30
	maxWindowDays       = 90
)

// Mount 注册个人用量与管理员看板/导出接口。
// 个人接口服务端强制 user_id = 当前用户，不接受任何用户参数；
// 管理端不提供按用户下钻（个人消耗仅本人可见）。
func Mount(rg *gin.RouterGroup, authSvc *auth.Service, s *Service) {
	rg.GET("/ai-usage", authSvc.AuthRequired(), s.personal)
	admin := rg.Group("/admin", authSvc.ChainMiddleware(auth.AdminRequired()))
	admin.GET("/ai-usage", s.admin)
	admin.GET("/ai-usage/export", s.export)
}

func (s *Service) personal(c *gin.Context) {
	uid := auth.CurrentUser(c).ID
	f, apiErr := s.parseFilter(c, personalDefaultDays, &uid)
	if apiErr != nil {
		httpapi.WriteError(c.Writer, apiErr)
		return
	}
	s.writeReport(c, f)
}

func (s *Service) admin(c *gin.Context) {
	f, apiErr := s.parseFilter(c, adminDefaultDays, nil)
	if apiErr != nil {
		httpapi.WriteError(c.Writer, apiErr)
		return
	}
	s.writeReport(c, f)
}

func (s *Service) writeReport(c *gin.Context, f Filter) {
	report, err := s.Aggregate(c.Request.Context(), f)
	if err != nil {
		slog.Error("aiusage: aggregate failed", "err", err)
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(report, ""))
}

func (s *Service) export(c *gin.Context) {
	f, apiErr := s.parseFilter(c, adminDefaultDays, nil)
	if apiErr != nil {
		httpapi.WriteError(c.Writer, apiErr)
		return
	}
	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "json" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	rows, err := s.ExportRows(c.Request.Context(), f)
	if err != nil {
		slog.Error("aiusage: export query failed", "err", err)
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
		return
	}
	filename := fmt.Sprintf("ai-usage-%s-%s.%s", f.From.In(s.Location).Format("20060102"), f.To.In(s.Location).Format("20060102"), format)
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	if format == "json" {
		c.Header("Content-Type", "application/json; charset=utf-8")
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(rows, ""))
		return
	}
	s.writeCSV(c, rows)
}

func (s *Service) writeCSV(c *gin.Context, rows []ExportRow) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	// BOM 让 Excel 正确识别 UTF-8（模型名/服务商名可能含中文）。
	_, _ = c.Writer.Write([]byte("\ufeff"))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"day", "provider", "model", "feature_kind", "calls", "success", "error", "timeout", "cancelled",
		"success_rate", "prompt_tokens", "completion_tokens", "usage_missing_calls",
		"latency_p50_ms", "latency_p95_ms", "first_token_p50_ms",
	})
	for _, r := range rows {
		_ = w.Write([]string{
			r.Day, r.Provider, r.Model, r.FeatureKind,
			strconv.FormatInt(r.Calls, 10), strconv.FormatInt(r.Success, 10),
			strconv.FormatInt(r.Error, 10), strconv.FormatInt(r.Timeout, 10),
			strconv.FormatInt(r.Cancelled, 10), strconv.FormatFloat(r.SuccessRate, 'f', 4, 64),
			strconv.FormatInt(r.PromptTokens, 10), strconv.FormatInt(r.CompletionTokens, 10),
			strconv.FormatInt(r.UsageMissingCalls, 10),
			nullableIntString(r.LatencyP50Ms), nullableIntString(r.LatencyP95Ms), nullableIntString(r.FirstTokenP50Ms),
		})
	}
	w.Flush()
}

func nullableIntString(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// parseFilter 解析 from/to/model/feature_kind。日期按 AI_USAGE_TIMEZONE 的本地日
// 解释，to 为闭区间上界；窗口默认按调用方（管理端 7 天 / 个人 30 天），上限 90 天。
func (s *Service) parseFilter(c *gin.Context, defaultDays int, userID *store.UUID) (Filter, *httpapi.APIError) {
	now := s.now().In(s.Location)
	f := Filter{UserID: userID}

	if v := strings.TrimSpace(c.Query("from")); v != "" {
		t, _, ok := parseTimeParam(v, s.Location)
		if !ok {
			return Filter{}, httpapi.ErrValidation
		}
		f.From = t
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		t, dateOnly, ok := parseTimeParam(v, s.Location)
		if !ok {
			return Filter{}, httpapi.ErrValidation
		}
		if dateOnly {
			t = t.AddDate(0, 0, 1) // 日期上界含当天
		}
		f.To = t
	}
	if f.To.IsZero() {
		f.To = now
	}
	if f.From.IsZero() {
		f.From = f.To.AddDate(0, 0, -defaultDays)
	}
	if !f.From.Before(f.To) {
		return Filter{}, httpapi.ErrValidation
	}
	if f.To.Sub(f.From) > time.Duration(maxWindowDays)*24*time.Hour {
		return Filter{}, httpapi.ErrValidation
	}

	if v := strings.TrimSpace(c.Query("model")); v != "" {
		if len([]rune(v)) > maxModelRunes {
			return Filter{}, httpapi.ErrValidation
		}
		f.Model = v
	}
	if v := strings.TrimSpace(c.Query("feature_kind")); v != "" {
		if len([]rune(v)) > maxFeatureRunes {
			return Filter{}, httpapi.ErrValidation
		}
		f.FeatureKind = v
	}
	return f, nil
}

func parseTimeParam(v string, loc *time.Location) (time.Time, bool, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, false, true
	}
	if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
		return t, true, true
	}
	return time.Time{}, false, false
}
