// Package settings 用户设置域：分类 JSONB 存储与默认值合并。
// 契约来源：backend/services/settings_service.py + schemas/user_settings.py + api/routes/settings.py
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// currentUserID 从认证中间件注入的用户取 UUID。
func currentUserID(c *gin.Context) uuid.UUID {
	return auth.CurrentUser(c).ID.UUID
}

// ValidCategories 对应 VALID_CATEGORIES。
var ValidCategories = []string{"ui", "editor", "agent"}

// DefaultSettings 对应 DEFAULT_SETTINGS（以 schemas/user_settings.py 代码为准）。
var DefaultSettings = map[string]map[string]any{
	"ui": {
		"theme":             "system",
		"language":          "zh-CN",
		"sidebar_collapsed": false,
	},
	"editor": {
		"font_size":          14,
		"tab_size":           2,
		"auto_save":          true,
		"auto_save_interval": 30,
		"word_wrap":          true,
	},
	"agent": {
		"providers":          []any{},
		"active_provider_id": "",
		"temperature":        0.7,
		"writing_style":      "academic",
		"memory_enabled":     true,
	},
}

func isValidCategory(cat string) bool {
	return cat == "ui" || cat == "editor" || cat == "agent"
}

// Service 设置域逻辑。
type Service struct {
	DB *gorm.DB
}

// NewService wires the settings domain.
func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// ValidationError 表示分类校验失败（对应 SettingsServiceError → 422）。
type ValidationError struct{ Msg string }

// UnknownCategoryError 对应 python "Unknown category: {cat}" 契约文案。
type UnknownCategoryError struct{ Category string }

func (e *UnknownCategoryError) Error() string { return "Unknown category: " + e.Category }

func (e *ValidationError) Error() string { return e.Msg }

// GetAll 读取全部分类并与默认值合并。
func (s *Service) GetAll(ctx context.Context, uid uuid.UUID) map[string]map[string]any {
	var rows []store.UserSettings
	_ = s.DB.WithContext(ctx).Where("user_id = ?", store.UUID{UUID: uid}).Find(&rows).Error
	stored := map[string]map[string]any{}
	for _, r := range rows {
		var m map[string]any
		_ = json.Unmarshal(r.Settings, &m)
		if m == nil {
			m = map[string]any{}
		}
		stored[r.Category] = m
	}
	merged := map[string]map[string]any{}
	for cat, def := range DefaultSettings {
		merged[cat] = shallowMerge(def, stored[cat])
	}
	return merged
}

// GetCategory 读取单分类（合并默认值）。
func (s *Service) GetCategory(ctx context.Context, uid uuid.UUID, cat string) map[string]any {
	var row store.UserSettings
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).First(&row).Error
	stored := map[string]any{}
	if err == nil {
		_ = json.Unmarshal(row.Settings, &stored)
	}
	return shallowMerge(DefaultSettings[cat], stored)
}

// UpdateCategory 合并更新（PATCH）：current+partial → 规范化校验 → upsert。
func (s *Service) UpdateCategory(ctx context.Context, uid uuid.UUID, cat string, partial map[string]any) (map[string]any, *ValidationError) {
	current := s.GetCategory(ctx, uid, cat)
	merged := shallowMerge(current, partial)
	return s.applyValidated(ctx, uid, cat, merged)
}

// ReplaceCategory 整体替换（PUT）：规范化校验后 upsert
// （Pydantic model_dump 会填充全部默认字段，故存储仍为完整字段集）。
func (s *Service) ReplaceCategory(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) (map[string]any, *ValidationError) {
	return s.applyValidated(ctx, uid, cat, data)
}

// ResetCategory 删除存储行（读取时回退默认值）。
func (s *Service) ResetCategory(ctx context.Context, uid uuid.UUID, cat string) {
	_ = s.DB.WithContext(ctx).
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).
		Delete(&store.UserSettings{}).Error
}

func (s *Service) applyValidated(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) (map[string]any, *ValidationError) {
	normalized, err := NormalizeCategory(cat, data)
	if err != nil {
		return nil, &ValidationError{Msg: err.Error()}
	}
	s.upsert(ctx, uid, cat, normalized)
	return s.GetCategory(ctx, uid, cat), nil
}

func (s *Service) upsert(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) {
	b, _ := json.Marshal(data)
	var row store.UserSettings
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).First(&row).Error
	if err == nil {
		row.Settings = datatypes.JSON(b)
		_ = s.DB.WithContext(ctx).Save(&row).Error
	} else {
		_ = s.DB.WithContext(ctx).Create(&store.UserSettings{
			UserID:   store.UUID{UUID: uid},
			Category: cat,
			Settings: datatypes.JSON(b),
		}).Error
	}
}

func shallowMerge(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// ── 分类校验（对应各 Pydantic schema 的约束语义）──

var langPattern = regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2,4}$`)

// NormalizeCategory 校验并归一化数据：未知字段忽略、缺失字段填默认、
// agent.providers 补生成 id（uuid hex 前 12 位）。
func NormalizeCategory(cat string, data map[string]any) (map[string]any, error) {
	switch cat {
	case "ui":
		return normalizeUI(data)
	case "editor":
		return normalizeEditor(data)
	case "agent":
		return normalizeAgent(data)
	default:
		return nil, &UnknownCategoryError{Category: cat}
	}
}

func pickBool(data map[string]any, key string, def bool) (bool, error) {
	v, ok := data[key]
	if !ok {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("field %s: must be a boolean", key)
	}
	return b, nil
}

func pickString(data map[string]any, key, def string) (string, error) {
	v, ok := data[key]
	if !ok {
		return def, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %s: must be a string", key)
	}
	return s, nil
}

func pickInt(data map[string]any, key string, def, min, max int) (int, error) {
	v, ok := data[key]
	if !ok {
		return def, nil
	}
	f, ok := asFloat(v)
	if !ok || f != float64(int(f)) {
		return 0, fmt.Errorf("field %s: must be an integer", key)
	}
	n := int(f)
	if n < min || n > max {
		return 0, fmt.Errorf("field %s: value out of range [%d, %d]", key, min, max)
	}
	return n, nil
}

// asFloat 归一 JSON 反序列化（float64）与 Go 直调（int/int64/float64）的数值。
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func normalizeUI(data map[string]any) (map[string]any, error) {
	theme, err := pickString(data, "theme", "system")
	if err != nil {
		return nil, err
	}
	switch theme {
	case "system", "light", "dark":
	default:
		return nil, fmt.Errorf("field theme: must be one of system/light/dark")
	}
	lang, err := pickString(data, "language", "zh-CN")
	if err != nil {
		return nil, err
	}
	if !langPattern.MatchString(lang) {
		return nil, fmt.Errorf("field language: invalid language tag")
	}
	collapsed, err := pickBool(data, "sidebar_collapsed", false)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"theme": theme, "language": lang, "sidebar_collapsed": collapsed,
	}, nil
}

func normalizeEditor(data map[string]any) (map[string]any, error) {
	font, err := pickInt(data, "font_size", 14, 8, 32)
	if err != nil {
		return nil, err
	}
	tab, err := pickInt(data, "tab_size", 2, 1, 8)
	if err != nil {
		return nil, err
	}
	autoSave, err := pickBool(data, "auto_save", true)
	if err != nil {
		return nil, err
	}
	interval, err := pickInt(data, "auto_save_interval", 30, 5, 300)
	if err != nil {
		return nil, err
	}
	wrap, err := pickBool(data, "word_wrap", true)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"font_size": font, "tab_size": tab, "auto_save": autoSave,
		"auto_save_interval": interval, "word_wrap": wrap,
	}, nil
}

func normalizeAgent(data map[string]any) (map[string]any, error) {
	providers := []any{}
	if raw, ok := data["providers"]; ok && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("field providers: must be a list")
		}
		for i, item := range list {
			p, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("field providers[%d]: must be an object", i)
			}
			norm, err := normalizeProvider(p)
			if err != nil {
				return nil, fmt.Errorf("field providers[%d]: %s", i, err)
			}
			providers = append(providers, norm)
		}
	}
	active, err := pickString(data, "active_provider_id", "")
	if err != nil {
		return nil, err
	}
	temp := 0.7
	if v, ok := data["temperature"]; ok {
		f, isNum := asFloat(v)
		if !isNum || f < 0.0 || f > 2.0 {
			return nil, fmt.Errorf("field temperature: must be a number in [0.0, 2.0]")
		}
		temp = f
	}
	style, err := pickString(data, "writing_style", "academic")
	if err != nil {
		return nil, err
	}
	if l := len(style); l < 1 || l > 50 {
		return nil, fmt.Errorf("field writing_style: length must be 1-50")
	}
	memory, err := pickBool(data, "memory_enabled", true)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"providers": providers, "active_provider_id": active,
		"temperature": temp, "writing_style": style, "memory_enabled": memory,
	}, nil
}

func normalizeProvider(p map[string]any) (map[string]any, error) {
	id, err := pickString(p, "id", "")
	if err != nil {
		return nil, err
	}
	if len(id) > 64 {
		return nil, fmt.Errorf("id too long")
	}
	if id == "" {
		id = strings.ReplaceAll(uuid.New().String(), "-", "")[:12] // uuid4().hex[:12]
	}
	name, err := pickString(p, "name", "")
	if err != nil {
		return nil, err
	}
	if len(name) > 100 {
		return nil, fmt.Errorf("name too long")
	}
	typ, err := pickString(p, "type", "openai_compatible")
	if err != nil {
		return nil, err
	}
	if typ != "openai_compatible" && typ != "anthropic" {
		return nil, fmt.Errorf("type must be openai_compatible or anthropic")
	}
	baseURL, err := pickString(p, "base_url", "")
	if err != nil {
		return nil, err
	}
	if len(baseURL) > 500 {
		return nil, fmt.Errorf("base_url too long")
	}
	apiKey, err := pickString(p, "api_key", "")
	if err != nil {
		return nil, err
	}
	if len(apiKey) > 500 {
		return nil, fmt.Errorf("api_key too long")
	}
	models := []any{}
	if raw, ok := p["models"]; ok && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("models must be a list")
		}
		models = list
	}
	active, err := pickString(p, "active_model", "")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "name": name, "type": typ, "base_url": baseURL,
		"api_key": apiKey, "models": models, "active_model": active,
	}, nil
}

// ── HTTP 路由 ──

// SettingsResponse / AllSettingsResponse 对应 schemas。
type SettingsResponse struct {
	Category string         `json:"category"`
	Settings map[string]any `json:"settings"`
}

type AllSettingsResponse struct {
	Settings map[string]map[string]any `json:"settings"`
}

type updateRequest struct {
	Settings map[string]any `json:"settings"`
}

// Mount 注册设置域路由（需挂认证中间件）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("/settings", authMw)

	g.GET("", func(c *gin.Context) {
		u := currentUserID(c)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(AllSettingsResponse{Settings: s.GetAll(c.Request.Context(), u)}, ""))
	})
	cat := func(c *gin.Context) string { return c.Param("category") }

	g.GET("/:category", func(c *gin.Context) {
		category := cat(c)
		if !isValidCategory(category) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Unknown settings category: " + category})
			return
		}
		u := currentUserID(c)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(SettingsResponse{Category: category, Settings: s.GetCategory(c.Request.Context(), u, category)}, ""))
	})

	writeValidationError := func(c *gin.Context, e *ValidationError) {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 422, Info: e.Msg})
	}

	patchPut := func(replace bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			category := cat(c)
			if !isValidCategory(category) {
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Unknown settings category: " + category})
				return
			}
			var req updateRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			if req.Settings == nil {
				req.Settings = map[string]any{}
			}
			u := currentUserID(c)
			var data map[string]any
			var verr *ValidationError
			if replace {
				data, verr = s.ReplaceCategory(c.Request.Context(), u, category, req.Settings)
			} else {
				data, verr = s.UpdateCategory(c.Request.Context(), u, category, req.Settings)
			}
			if verr != nil {
				writeValidationError(c, verr)
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(SettingsResponse{Category: category, Settings: data}, ""))
		}
	}
	g.PATCH("/:category", patchPut(false))
	g.PUT("/:category", patchPut(true))

	g.DELETE("/:category", func(c *gin.Context) {
		category := cat(c)
		if !isValidCategory(category) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Unknown settings category: " + category})
			return
		}
		u := currentUserID(c)
		s.ResetCategory(c.Request.Context(), u, category)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(SettingsResponse{Category: category, Settings: s.GetCategory(c.Request.Context(), u, category)}, ""))
	})
}
