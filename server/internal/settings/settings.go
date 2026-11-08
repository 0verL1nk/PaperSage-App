// Package settings 用户设置域：分类 JSONB 存储与默认值合并。
// 契约来源：backend/services/settings_service.py + schemas/user_settings.py + api/routes/settings.py
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent/opencode"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
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
	DB  *gorm.DB
	Cfg *config.Settings
}

// NewService wires the settings domain.
func NewService(db *gorm.DB, cfg *config.Settings) *Service {
	return &Service{DB: db, Cfg: cfg}
}

// ValidationError 表示分类校验失败（对应 SettingsServiceError → 422）。
type ValidationError struct {
	Msg      string
	internal bool
}

// UnknownCategoryError 对应 python "Unknown category: {cat}" 契约文案。
type UnknownCategoryError struct{ Category string }

func (e *UnknownCategoryError) Error() string { return "Unknown category: " + e.Category }

func (e *ValidationError) Error() string { return e.Msg }

// GetAll 读取全部分类并与默认值合并。
func (s *Service) GetAll(ctx context.Context, uid uuid.UUID) (map[string]map[string]any, error) {
	var rows []store.UserSettings
	if err := s.DB.WithContext(ctx).Where("user_id = ?", store.UUID{UUID: uid}).Find(&rows).Error; err != nil {
		return nil, err
	}
	stored := map[string]map[string]any{}
	for _, r := range rows {
		var m map[string]any
		if err := json.Unmarshal(r.Settings, &m); err != nil || m == nil {
			return nil, errors.New("invalid saved settings")
		}
		stored[r.Category] = m
	}
	merged := map[string]map[string]any{}
	for cat, def := range DefaultSettings {
		merged[cat] = shallowMerge(def, stored[cat])
	}
	return merged, nil
}

// GetCategory 读取单分类（合并默认值）。
func (s *Service) GetCategory(ctx context.Context, uid uuid.UUID, cat string) (map[string]any, error) {
	var row store.UserSettings
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).First(&row).Error
	stored := map[string]any{}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if json.Unmarshal(row.Settings, &stored) != nil || stored == nil {
			return nil, errors.New("invalid saved settings")
		}
	}
	return shallowMerge(DefaultSettings[cat], stored), nil
}

// UpdateCategory 合并更新（PATCH）：current+partial → 规范化校验 → upsert。
func (s *Service) UpdateCategory(ctx context.Context, uid uuid.UUID, cat string, partial map[string]any) (map[string]any, *ValidationError) {
	var normalized map[string]any
	db := s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
	err := db.Transaction(func(tx *gorm.DB) error {
		// Insert-if-absent also obtains SQLite's write lock before reading.
		seed := store.UserSettings{UserID: store.UUID{UUID: uid}, Category: cat, Settings: datatypes.JSON([]byte(`{}`))}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var row store.UserSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).First(&row).Error; err != nil {
			return err
		}
		stored := map[string]any{}
		if err := json.Unmarshal(row.Settings, &stored); err != nil || stored == nil {
			return errors.New("invalid saved settings")
		}
		merged := shallowMerge(shallowMerge(DefaultSettings[cat], stored), partial)
		if selection, ok := partial["model_selection"]; ok {
			if cat != "agent" || len(partial) != 1 {
				return &ValidationError{Msg: "model_selection must be the only field in an agent patch"}
			}
			if err := applyModelSelection(merged, selection); err != nil {
				return err
			}
			delete(merged, "model_selection")
		}
		var err error
		if _, selecting := partial["model_selection"]; selecting {
			normalized, err = normalizeAgent(merged, true)
		} else {
			normalized, err = NormalizeCategory(cat, merged)
		}
		if err != nil {
			return &ValidationError{Msg: err.Error()}
		}
		if _, selecting := partial["model_selection"]; selecting {
			// Keep legacy provider fields unchanged after validation.
			normalized["providers"] = merged["providers"]
		}
		b, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		return tx.Model(&row).Update("settings", datatypes.JSON(b)).Error
	})
	if err != nil {
		var validation *ValidationError
		if errors.As(err, &validation) {
			return nil, validation
		}
		return nil, &ValidationError{Msg: "Failed to save settings", internal: true}
	}
	return normalized, nil
}

func applyModelSelection(settings map[string]any, raw any) *ValidationError {
	selection, ok := raw.(map[string]any)
	if !ok || len(selection) != 2 {
		return &ValidationError{Msg: "model_selection must contain only provider_id and model"}
	}
	id, idOK := selection["provider_id"].(string)
	model, modelOK := selection["model"].(string)
	if !idOK || strings.TrimSpace(id) == "" || !modelOK || strings.TrimSpace(model) == "" {
		return &ValidationError{Msg: "model_selection provider_id and model must be non-empty strings"}
	}
	providers, ok := settings["providers"].([]any)
	if !ok {
		return &ValidationError{Msg: "field providers: must be a list"}
	}
	seen := make(map[string]bool, len(providers))
	var selected map[string]any
	for _, item := range providers {
		provider, ok := item.(map[string]any)
		if !ok {
			return &ValidationError{Msg: "field providers: entries must be objects"}
		}
		providerID, ok := provider["id"].(string)
		if !ok || strings.TrimSpace(providerID) == "" || seen[providerID] {
			return &ValidationError{Msg: "provider IDs must be unique non-empty strings"}
		}
		seen[providerID] = true
		if providerID == id {
			selected = provider
		}
	}
	if selected == nil {
		return &ValidationError{Msg: "model_selection provider_id must identify a provider"}
	}
	selected["active_model"] = model
	settings["active_provider_id"] = id
	return nil
}

// ReplaceCategory 整体替换（PUT）：规范化校验后 upsert
// （Pydantic model_dump 会填充全部默认字段，故存储仍为完整字段集）。
func (s *Service) ReplaceCategory(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) (map[string]any, *ValidationError) {
	if _, ok := data["model_selection"]; ok {
		return nil, &ValidationError{Msg: "model_selection is only supported in an agent patch"}
	}
	return s.applyValidated(ctx, uid, cat, data)
}

// ResetCategory 删除存储行（读取时回退默认值）。
func (s *Service) ResetCategory(ctx context.Context, uid uuid.UUID, cat string) error {
	return s.DB.WithContext(ctx).
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).
		Delete(&store.UserSettings{}).Error
}

func (s *Service) applyValidated(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) (map[string]any, *ValidationError) {
	normalized, err := NormalizeCategory(cat, data)
	if err != nil {
		return nil, &ValidationError{Msg: err.Error()}
	}
	if err := s.upsert(ctx, uid, cat, normalized); err != nil {
		return nil, &ValidationError{Msg: "Failed to save settings", internal: true}
	}
	return normalized, nil
}

func (s *Service) upsert(ctx context.Context, uid uuid.UUID, cat string, data map[string]any) error {
	b, _ := json.Marshal(data)
	// SQL log interpolation would expose provider API keys on failed writes.
	db := s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
	var row store.UserSettings
	err := db.
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, cat).First(&row).Error
	if err == nil {
		row.Settings = datatypes.JSON(b)
		return db.Save(&row).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(&store.UserSettings{
		UserID:   store.UUID{UUID: uid},
		Category: cat,
		Settings: datatypes.JSON(b),
	}).Error
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
		return normalizeAgent(data, false)
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

func normalizeAgent(data map[string]any, allowLegacyEmptyModel bool) (map[string]any, error) {
	providers := []any{}
	ids := map[string]bool{}
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
			norm, err := normalizeProvider(p, allowLegacyEmptyModel)
			if err != nil {
				return nil, fmt.Errorf("field providers[%d]: %s", i, err)
			}
			id := norm["id"].(string)
			if ids[id] {
				return nil, fmt.Errorf("provider IDs must be unique")
			}
			ids[id] = true
			providers = append(providers, norm)
		}
	}
	active, err := pickString(data, "active_provider_id", "")
	if err != nil {
		return nil, err
	}
	if active != "" {
		found := false
		for _, provider := range providers {
			if provider.(map[string]any)["id"] == active {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("active_provider_id must identify a provider")
		}
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

func normalizeProvider(p map[string]any, allowLegacyEmptyModel bool) (map[string]any, error) {
	id, err := pickString(p, "id", "")
	if err != nil {
		return nil, err
	}
	if len(id) > 64 {
		return nil, fmt.Errorf("id too long")
	}
	if id == "" {
		id = strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	} else if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id must not be blank")
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
		for _, item := range list {
			model, ok := item.(string)
			if !ok || strings.TrimSpace(model) == "" {
				return nil, fmt.Errorf("models must contain non-empty strings")
			}
		}
		models = list
	}
	defaultModel := ""
	if _, exists := p["active_model"]; !exists && len(models) > 0 {
		defaultModel, _ = models[len(models)-1].(string)
	}
	active, err := pickString(p, "active_model", defaultModel)
	if err != nil {
		return nil, err
	}
	if (len(models) > 0 || active != "") && (!allowLegacyEmptyModel || active != "") {
		valid := false
		for _, model := range models {
			if model == active {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("active_model must be one of models")
		}
	}
	return map[string]any{
		"id": id, "name": name, "type": typ, "base_url": baseURL,
		"api_key": apiKey, "models": models, "active_model": active,
	}, nil
}

// ── 服务端兜底与连接测试 ──

// ServerFallback reports whether a server-side model is available when the
// user has configured no provider. Admin settings win over env, matching
// agent.ResolveModelWithEnv. The API key is never exposed.
type ServerFallback struct {
	Configured bool   `json:"configured"`
	Model      string `json:"model"`
}

// ServerFallback reads admin settings, then env.
func (s *Service) ServerFallback() ServerFallback {
	if s.DB != nil {
		if a := syssettings.GetAgent(s.DB); a.APIKey != "" && a.BaseURL != "" {
			model := a.Model
			if model == "" && s.Cfg != nil {
				model = s.Cfg.AgentModel
			}
			return ServerFallback{Configured: true, Model: model}
		}
	}
	if s.Cfg != nil && s.Cfg.AgentAPIKey != "" && s.Cfg.AgentBaseURL != "" {
		return ServerFallback{Configured: true, Model: s.Cfg.AgentModel}
	}
	return ServerFallback{}
}

// withServerFallback injects the read-only status into agent responses. It is
// added after normalization, so it is never persisted.
func (s *Service) withServerFallback(category string, data map[string]any) map[string]any {
	if category != "agent" {
		return data
	}
	if data == nil {
		data = map[string]any{}
	}
	data["server_fallback"] = s.ServerFallback()
	return data
}

const agentTestTimeout = 10 * time.Second

// AgentTestRequest is the connection-check payload. The key is used only for
// the outbound probe and is never echoed or logged.
type AgentTestRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// AgentTestResult is returned when the probe succeeds.
type AgentTestResult struct {
	OK     bool     `json:"ok"`
	Models []string `json:"models,omitempty"`
}

type providerHTTPError struct{ status int }

func (e providerHTTPError) Error() string {
	return fmt.Sprintf("provider returned HTTP %d", e.status)
}

// TestAgentConnection probes an OpenAI-compatible endpoint: GET /models first,
// then a one-token chat completion. The 10s budget covers both attempts.
func (s *Service) TestAgentConnection(ctx context.Context, req AgentTestRequest) (*AgentTestResult, error) {
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	apiKey := strings.TrimSpace(req.APIKey)
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("base_url must be a valid http(s) URL")
	}
	ctx, cancel := context.WithTimeout(ctx, agentTestTimeout)
	defer cancel()
	client := testClientFactory(base)

	models, err := fetchProviderModels(ctx, client, base, apiKey)
	if err == nil {
		return &AgentTestResult{OK: true, Models: models}, nil
	}
	var httpErr providerHTTPError
	if !errors.As(err, &httpErr) {
		return nil, providerTransportError(err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, httpErr
	}
	if err := probeProviderChat(ctx, client, base, apiKey, model); err != nil {
		var chatErr providerHTTPError
		if !errors.As(err, &chatErr) {
			return nil, providerTransportError(err)
		}
		return nil, chatErr
	}
	return &AgentTestResult{OK: true}, nil
}

// testClientFactory is swapped by tests that probe httptest loopback servers.
var testClientFactory = newAgentTestClient

// publicOnlyControl rejects targets that resolve to private, loopback or
// link-local addresses so the connection probe cannot be used as an SSRF
// scanner against the server's own network or cloud metadata.
func publicOnlyControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("provider host did not resolve to an IP")
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return errors.New("provider host resolves to a private address")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("provider host resolves to a private address")
	}
	return nil
}

func newAgentTestClient(baseURL string) *http.Client {
	dial := (&net.Dialer{Timeout: agentTestTimeout, Control: publicOnlyControl}).DialContext
	var rt http.RoundTripper
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		base = base.Clone()
		base.DialContext = dial
		rt = base
	} else {
		rt = &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dial}
	}
	if opencode.IsGateway(baseURL) {
		rt = opencode.SessionTransport(rt, uuid.NewString())
	}
	return &http.Client{Transport: rt, Timeout: agentTestTimeout}
}

func fetchProviderModels(ctx context.Context, client *http.Client, base, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providerHTTPError{status: resp.StatusCode}
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	// A 2xx response already proves connectivity; an unusual body is not a failure.
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return nil, nil
	}
	models := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			models = append(models, id)
		}
	}
	return models, nil
}

func probeProviderChat(ctx context.Context, client *http.Client, base, apiKey, model string) error {
	payload, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerHTTPError{status: resp.StatusCode}
	}
	return nil
}

func providerTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("connection timed out")
	}
	return errors.New("could not connect to provider")
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
		data, err := s.GetAll(c.Request.Context(), u)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Failed to load settings"})
			return
		}
		data["agent"] = s.withServerFallback("agent", data["agent"])
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(AllSettingsResponse{Settings: data}, ""))
	})

	// Probe an OpenAI-compatible endpoint with the user's draft credentials.
	// The key is never logged or echoed back.
	g.POST("/agent/test", func(c *gin.Context) {
		var req AgentTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		if strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.APIKey) == "" {
			httpapi.WriteError(c.Writer, httpapi.BadRequest("base_url and api_key are required"))
			return
		}
		result, err := s.TestAgentConnection(c.Request.Context(), req)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusBadGateway, Info: err.Error()})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(result, ""))
	})
	cat := func(c *gin.Context) string { return c.Param("category") }

	g.GET("/:category", func(c *gin.Context) {
		category := cat(c)
		if !isValidCategory(category) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Unknown settings category: " + category})
			return
		}
		u := currentUserID(c)
		data, err := s.GetCategory(c.Request.Context(), u, category)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Failed to load settings"})
			return
		}
		data = s.withServerFallback(category, data)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(SettingsResponse{Category: category, Settings: data}, ""))
	})

	writeValidationError := func(c *gin.Context, e *ValidationError) {
		if e.internal {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: e.Msg})
			return
		}
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
			data = s.withServerFallback(category, data)
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
		if err := s.ResetCategory(c.Request.Context(), u, category); err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Failed to reset settings"})
			return
		}
		settings := s.withServerFallback(category, shallowMerge(DefaultSettings[category], nil))
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(SettingsResponse{Category: category, Settings: settings}, ""))
	})
}
