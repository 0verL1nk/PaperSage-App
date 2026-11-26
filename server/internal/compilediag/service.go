// Package compilediag 接收浏览器 WASM 编译失败诊断，并提供管理员查询。
//
// 隐私红线：只收结构化诊断 + 白名单脱敏后的日志片段，绝不上传论文正文；
// 客户端先脱敏，服务端再强制脱敏一次（不信任客户端）。本表只追加、按保留期
// 硬删，与用户可见的 compile_jobs（编译历史）职责分离。
package compilediag

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/ratelimit"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	maxPayloadBytes      = 128 << 10
	maxFailedResources   = 20
	maxResourceNameRunes = 200
	maxResourceErrRunes  = 300
	maxEngineFieldRunes  = 64
	maxErrorSummaryRunes = 500

	userPerMinute = 6
	userPerHour   = 30
	guestPerMin   = 3
	guestPerHour  = 10

	cleanupInterval = time.Hour

	defaultPageSize = 20
	maxPageSize     = 100

	defaultRetentionDays = 30
)

var (
	validFailureKinds = map[string]bool{
		"resource": true, "timeout": true, "service": true, "latex": true, "unknown": true,
	}
	validEngines = map[string]bool{"pdflatex": true, "xelatex": true}
)

// Service 持有 DB、限流器与保留期配置。
type Service struct {
	DB            *gorm.DB
	RetentionDays int

	userLimiter *ratelimit.Limiter
	ipLimiter   *ratelimit.Limiter

	mu          sync.Mutex
	lastCleanup time.Time
	now         func() time.Time
}

func NewService(db *gorm.DB, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = defaultRetentionDays
	}
	return &Service{
		DB:            db,
		RetentionDays: retentionDays,
		userLimiter:   ratelimit.New(),
		ipLimiter:     ratelimit.New(),
		now:           time.Now,
	}
}

type clientEnv struct {
	UserAgent           string `json:"user_agent"`
	Platform            string `json:"platform"`
	Language            string `json:"language"`
	HardwareConcurrency int    `json:"hardware_concurrency"`
	DeviceMemoryGB      int    `json:"device_memory_gb"`
	Online              bool   `json:"online"`
}

type failedResource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Status *int   `json:"status"`
	Error  string `json:"error"`
}

type ingestRequest struct {
	CompileJobID    *string          `json:"compile_job_id"`
	ProjectID       *string          `json:"project_id"`
	ClientKind      string           `json:"client_kind"`
	Engine          string           `json:"engine"`
	EngineVersion   string           `json:"engine_version"`
	ResourceProfile string           `json:"resource_profile"`
	FailureKind     string           `json:"failure_kind"`
	FailureStage    string           `json:"failure_stage"`
	DurationMS      int              `json:"duration_ms"`
	HTTPStatus      *int             `json:"http_status"`
	ErrorMessage    string           `json:"error_message"`
	LogExcerpt      string           `json:"log_excerpt"`
	FailedResources []failedResource `json:"failed_resources"`
	ClientEnv       *clientEnv       `json:"client_env"`
}

// Mount 注册公开上报接口与管理员查询接口。
func Mount(rg *gin.RouterGroup, authSvc *auth.Service, s *Service) {
	rg.POST("/compile-diagnostics", authSvc.OptionalAuth(), s.ingest)

	admin := rg.Group("/admin", authSvc.ChainMiddleware(auth.AdminRequired()))
	admin.GET("/compile-diagnostics", s.list)
	admin.GET("/compile-diagnostics/:id", s.detail)
}

func (s *Service) ingest(c *gin.Context) {
	user := auth.CurrentUser(c)
	if !s.allow(user, c.ClientIP()) {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusTooManyRequests, Info: "Too many requests"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPayloadBytes)
	var req ingestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusRequestEntityTooLarge, Info: "Payload too large"})
			return
		}
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if !validEngines[req.Engine] {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	ctx := c.Request.Context()
	report := &store.CompileErrorReport{
		ClientKind:      normalizeClientKind(req.ClientKind),
		Engine:          req.Engine,
		EngineVersion:   truncateRunes(req.EngineVersion, maxEngineFieldRunes),
		ResourceProfile: truncateRunes(req.ResourceProfile, maxEngineFieldRunes),
		FailureKind:     normalizeFailureKind(req.FailureKind),
		FailureStage:    normalizeFailureStage(req.FailureStage),
		DurationMS:      max(req.DurationMS, 0),
		ErrorMessage:    RedactErrorMessage(normalizeFailureKind(req.FailureKind), req.ErrorMessage),
	}
	if report.ErrorMessage == "" {
		report.ErrorMessage = "编译失败（客户端未提供摘要）"
	}
	if req.HTTPStatus != nil {
		status := int16(*req.HTTPStatus)
		report.HTTPStatus = &status
	}
	if report.FailureKind == "latex" {
		if excerpt := RedactLog(req.LogExcerpt); excerpt != "" {
			report.LogExcerpt = &excerpt
		}
	}
	if resources := sanitizeResources(req.FailedResources); len(resources) > 0 {
		report.FailedResources = marshalJSON(resources)
	}
	if req.ClientEnv != nil {
		report.ClientEnv = marshalJSON(req.ClientEnv)
	}

	if user == nil {
		// 游客：不落 IP，不绑定任何用户/项目。
		report.IsGuest = true
	} else {
		report.UserID = &store.UUID{UUID: user.ID.UUID}
		report.ProjectID = s.ownedProjectID(ctx, user.ID.UUID, req.ProjectID)
		report.CompileJobID = s.ownedJobID(ctx, user.ID.UUID, req.CompileJobID)
	}

	if err := s.DB.WithContext(ctx).Create(report).Error; err != nil {
		slog.Error("compilediag: persist report failed", "err", err)
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
		return
	}
	s.maybeCleanup(ctx)

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"id": report.ID.String()}, ""))
}

// allow 按登录态分档限流；游客只按 IP（不落库）。
func (s *Service) allow(user *store.User, ip string) bool {
	if user != nil {
		id := user.ID.String()
		return s.userLimiter.AllowWindow("diag:user:"+id, userPerMinute, time.Minute) &&
			s.userLimiter.AllowWindow("diag:user-hour:"+id, userPerHour, time.Hour)
	}
	return s.ipLimiter.AllowWindow("diag:ip:"+ip, guestPerMin, time.Minute) &&
		s.ipLimiter.AllowWindow("diag:ip-hour:"+ip, guestPerHour, time.Hour)
}

// ownedProjectID / ownedJobID 校验归属：不属于本人则置 NULL 并告警，
// 既不丢诊断，也不允许把记录挂到别人的项目上。
func (s *Service) ownedProjectID(ctx context.Context, uid uuid.UUID, raw *string) *store.UUID {
	if raw == nil || *raw == "" {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	var count int64
	err = s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: id}, store.UUID{UUID: uid}).
		Count(&count).Error
	if err != nil || count == 0 {
		slog.Warn("compilediag: project not owned by reporter, dropping link", "user", uid, "project", id)
		return nil
	}
	return &store.UUID{UUID: id}
}

func (s *Service) ownedJobID(ctx context.Context, uid uuid.UUID, raw *string) *store.UUID {
	if raw == nil || *raw == "" {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	var count int64
	err = s.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("id = ? AND user_id = ?", store.UUID{UUID: id}, store.UUID{UUID: uid}).
		Count(&count).Error
	if err != nil || count == 0 {
		slog.Warn("compilediag: compile job not owned by reporter, dropping link", "user", uid, "job", id)
		return nil
	}
	return &store.UUID{UUID: id}
}

// maybeCleanup 沿用请求路径惰性清理（与 ExpireStaleRunningJobs 同模式）：
// 距上次清理 ≥1 小时时同步删过期行，不引入定时器。
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
	res := s.DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&store.CompileErrorReport{})
	if res.Error != nil {
		slog.Warn("compilediag: retention cleanup failed", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("compilediag: retention cleanup removed expired reports", "rows", res.RowsAffected)
	}
}

func sanitizeResources(in []failedResource) []failedResource {
	if len(in) == 0 {
		return nil
	}
	if len(in) > maxFailedResources {
		in = in[:maxFailedResources]
	}
	out := make([]failedResource, 0, len(in))
	for _, r := range in {
		out = append(out, failedResource{
			Kind:   truncateRunes(r.Kind, 32),
			Name:   truncateRunes(r.Name, maxResourceNameRunes),
			State:  truncateRunes(r.State, 32),
			Status: r.Status,
			Error:  truncateRunes(r.Error, maxResourceErrRunes),
		})
	}
	return out
}

func marshalJSON(v any) datatypes.JSON {
	b, err := jsonMarshal(v)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func normalizeClientKind(v string) string {
	if v == "desktop" {
		return "desktop"
	}
	return "web"
}

func normalizeFailureKind(v string) string {
	if validFailureKinds[v] {
		return v
	}
	return "unknown"
}

func normalizeFailureStage(v string) string {
	switch v {
	case "prepare", "job_create", "compile", "upload":
		return v
	}
	return "compile"
}

func truncateRunes(v string, max int) string {
	runes := []rune(strings.TrimSpace(v))
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// ── 管理员查询 ────────────────────────────────────────────────

type adminItem struct {
	ID              store.UUID     `json:"id" gorm:"column:id"`
	CreatedAt       time.Time      `json:"created_at" gorm:"column:created_at"`
	UserID          *store.UUID    `json:"user_id" gorm:"column:user_id"`
	Username        *string        `json:"username" gorm:"column:username"`
	ProjectID       *store.UUID    `json:"project_id" gorm:"column:project_id"`
	ProjectName     *string        `json:"project_name" gorm:"column:project_name"`
	IsGuest         bool           `json:"is_guest" gorm:"column:is_guest"`
	ClientKind      string         `json:"client_kind" gorm:"column:client_kind"`
	Engine          string         `json:"engine" gorm:"column:engine"`
	EngineVersion   string         `json:"engine_version" gorm:"column:engine_version"`
	ResourceProfile string         `json:"resource_profile" gorm:"column:resource_profile"`
	FailureKind     string         `json:"failure_kind" gorm:"column:failure_kind"`
	FailureStage    string         `json:"failure_stage" gorm:"column:failure_stage"`
	DurationMS      int            `json:"duration_ms" gorm:"column:duration_ms"`
	HTTPStatus      *int16         `json:"http_status" gorm:"column:http_status"`
	ErrorMessage    string         `json:"error_message" gorm:"column:error_message"`
	FailedResources datatypes.JSON `json:"failed_resources" gorm:"column:failed_resources"`
}

type adminDetail struct {
	ID              store.UUID     `json:"id" gorm:"column:id"`
	CreatedAt       time.Time      `json:"created_at" gorm:"column:created_at"`
	UserID          *store.UUID    `json:"user_id" gorm:"column:user_id"`
	Username        *string        `json:"username" gorm:"column:username"`
	ProjectID       *store.UUID    `json:"project_id" gorm:"column:project_id"`
	ProjectName     *string        `json:"project_name" gorm:"column:project_name"`
	IsGuest         bool           `json:"is_guest" gorm:"column:is_guest"`
	ClientKind      string         `json:"client_kind" gorm:"column:client_kind"`
	Engine          string         `json:"engine" gorm:"column:engine"`
	EngineVersion   string         `json:"engine_version" gorm:"column:engine_version"`
	ResourceProfile string         `json:"resource_profile" gorm:"column:resource_profile"`
	FailureKind     string         `json:"failure_kind" gorm:"column:failure_kind"`
	FailureStage    string         `json:"failure_stage" gorm:"column:failure_stage"`
	DurationMS      int            `json:"duration_ms" gorm:"column:duration_ms"`
	HTTPStatus      *int16         `json:"http_status" gorm:"column:http_status"`
	ErrorMessage    string         `json:"error_message" gorm:"column:error_message"`
	FailedResources datatypes.JSON `json:"failed_resources" gorm:"column:failed_resources"`
	CompileJobID    *store.UUID    `json:"compile_job_id" gorm:"column:compile_job_id"`
	LogExcerpt      *string        `json:"log_excerpt" gorm:"column:log_excerpt"`
	ClientEnv       datatypes.JSON `json:"client_env" gorm:"column:client_env"`
}

type listFilters struct {
	Page        int
	PageSize    int
	FailureKind string
	Engine      string
	IsGuest     *bool
	UserID      *store.UUID
	ProjectID   *store.UUID
	From        *time.Time
	// To 是闭区间上界；日期形式（YYYY-MM-DD）转为次日零点并用 < 比较。
	To         *time.Time
	ToDateOnly bool
}

func parseFilters(c *gin.Context) (*listFilters, *httpapi.APIError) {
	f := &listFilters{Page: 1, PageSize: defaultPageSize}
	if v := c.Query("page"); v != "" {
		n, err := parsePositiveInt(v)
		if err != nil {
			return nil, httpapi.ErrValidation
		}
		f.Page = n
	}
	if v := c.Query("page_size"); v != "" {
		n, err := parsePositiveInt(v)
		if err != nil {
			return nil, httpapi.ErrValidation
		}
		if n > maxPageSize {
			n = maxPageSize
		}
		f.PageSize = n
	}
	if v := c.Query("failure_kind"); v != "" {
		if !validFailureKinds[v] {
			return nil, httpapi.ErrValidation
		}
		f.FailureKind = v
	}
	if v := c.Query("engine"); v != "" {
		if !validEngines[v] {
			return nil, httpapi.ErrValidation
		}
		f.Engine = v
	}
	if v := c.Query("is_guest"); v != "" {
		b, ok := parseBool(v)
		if !ok {
			return nil, httpapi.ErrValidation
		}
		f.IsGuest = &b
	}
	if v := c.Query("user_id"); v != "" {
		id, ok := parseUUID(v)
		if !ok {
			return nil, httpapi.ErrValidation
		}
		f.UserID = id
	}
	if v := c.Query("project_id"); v != "" {
		id, ok := parseUUID(v)
		if !ok {
			return nil, httpapi.ErrValidation
		}
		f.ProjectID = id
	}
	if v := c.Query("from"); v != "" {
		t, dateOnly, ok := parseTimeParam(v)
		if !ok {
			return nil, httpapi.ErrValidation
		}
		_ = dateOnly
		f.From = &t
	}
	if v := c.Query("to"); v != "" {
		t, dateOnly, ok := parseTimeParam(v)
		if !ok {
			return nil, httpapi.ErrValidation
		}
		f.To = &t
		f.ToDateOnly = dateOnly
	}
	return f, nil
}

func (s *Service) list(c *gin.Context) {
	f, apiErr := parseFilters(c)
	if apiErr != nil {
		httpapi.WriteError(c.Writer, apiErr)
		return
	}

	q := s.DB.WithContext(c.Request.Context()).
		Table("compile_error_reports AS r").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Joins("LEFT JOIN projects p ON p.id = r.project_id")
	q = applyFilters(q, f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
		return
	}

	items := make([]adminItem, 0)
	if total > 0 {
		if err := q.Select(`r.id, r.created_at, r.user_id, u.username,
				r.project_id, p.name AS project_name, r.is_guest, r.client_kind,
				r.engine, r.engine_version, r.resource_profile, r.failure_kind,
				r.failure_stage, r.duration_ms, r.http_status, r.error_message,
				r.failed_resources`).
			Order("r.created_at DESC, r.id DESC").
			Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
			Scan(&items).Error; err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
			return
		}
	}

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"items": items, "total": total, "page": f.Page, "page_size": f.PageSize,
	}, ""))
}

func (s *Service) detail(c *gin.Context) {
	id, ok := parseUUID(c.Param("id"))
	if !ok {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var row adminDetail
	err := s.DB.WithContext(c.Request.Context()).
		Table("compile_error_reports AS r").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Joins("LEFT JOIN projects p ON p.id = r.project_id").
		Select(`r.id, r.created_at, r.user_id, u.username,
			r.project_id, p.name AS project_name, r.is_guest, r.client_kind,
			r.engine, r.engine_version, r.resource_profile, r.failure_kind,
			r.failure_stage, r.duration_ms, r.http_status, r.error_message,
			r.failed_resources, r.compile_job_id, r.log_excerpt, r.client_env`).
		Where("r.id = ?", *id).
		Scan(&row).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "Internal server error"})
		return
	}
	if row.ID.UUID == uuid.Nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Compile diagnostic"))
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(row, ""))
}
