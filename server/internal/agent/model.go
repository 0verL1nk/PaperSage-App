// Package agent — AI 写作助手域（Eino ADK，ReAct 工具循环）。
//
// 模型解析链：用户设置 agent.providers[active_provider_id]（BYO key）
// → 服务端 env（AGENT_API_KEY/BASE_URL/MODEL）兜底；都没有则报错，
// 前端引导到设置页。模型实例按请求构建，无全局态。
package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

// ProviderConfig 是 settings.agent.providers[] 的单条形状
// （与前端 AgentSettings/model-selector 契约一致）。
type ProviderConfig struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	BaseURL     string          `json:"base_url"`
	APIKey      string          `json:"api_key"`
	Models      []string        `json:"models"`
	ActiveModel json.RawMessage `json:"active_model"`
	IsActive    bool            `json:"is_active,omitempty"`
}

// ResolvedModel 是解析后的可用模型配置。
type ResolvedModel struct {
	Source  string // "user-settings" | "server-env"
	Name    string // 人类可读（provider 名或 env model）
	Model   string // 模型 ID
	BaseURL string
	APIKey  string
}

// ErrNoModel 未配置任何可用模型。
var ErrNoModel = fmt.Errorf("no model configured")

// ErrInvalidModel prevents an explicit user selection from falling back silently.
var ErrInvalidModel = fmt.Errorf("selected model configuration is invalid")

// ResolveModel 按链解析：user settings → env。
func ResolveModel(ctx context.Context, db *gorm.DB, userID interface{ String() string }) (*ResolvedModel, error) {
	// 1) 用户设置：使用与写入一致的 UUID 数据库表示。
	uid, err := uuid.Parse(userID.String())
	if err != nil {
		return nil, ErrInvalidModel
	}
	raw := db.WithContext(ctx).
		Table("user_settings").
		Select("settings").
		Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, "agent").
		Row()
	var buf []byte
	if err := raw.Scan(&buf); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoModel
		}
		return nil, fmt.Errorf("cannot read model settings")
	}
	if len(buf) > 0 {
		var s struct {
			Providers        []ProviderConfig `json:"providers"`
			ActiveProviderID string           `json:"active_provider_id"`
		}
		if json.Unmarshal(buf, &s) == nil {
			for _, p := range s.Providers {
				if s.ActiveProviderID != "" && p.ID != s.ActiveProviderID {
					continue
				}
				if strings.TrimSpace(p.APIKey) == "" || strings.TrimSpace(p.BaseURL) == "" || len(p.Models) == 0 {
					if s.ActiveProviderID != "" {
						return nil, ErrInvalidModel
					}
					continue
				}
				// Older settings without active_model used the last configured model.
				m := p.Models[len(p.Models)-1]
				if p.ActiveModel != nil {
					if string(p.ActiveModel) == "null" || json.Unmarshal(p.ActiveModel, &m) != nil {
						return nil, ErrInvalidModel
					}
				}
				valid := false
				for _, candidate := range p.Models {
					if candidate == m && strings.TrimSpace(m) != "" {
						valid = true
						break
					}
				}
				if !valid {
					return nil, ErrInvalidModel
				}
				return &ResolvedModel{Source: "user-settings", Name: p.Name, Model: m, BaseURL: p.BaseURL, APIKey: p.APIKey}, nil
			}
			if s.ActiveProviderID != "" {
				return nil, ErrInvalidModel
			}
		} else {
			return nil, ErrInvalidModel
		}
	}
	return nil, ErrNoModel
}

// ResolveModelWithEnv 完整解析链：
// 用户设置（BYO key）→ 管理后台（system_settings.agent，热生效）→ env 兜底。
func ResolveModelWithEnv(ctx context.Context, db *gorm.DB, userID interface{ String() string }, cfg *config.Settings) (*ResolvedModel, error) {
	if m, err := ResolveModel(ctx, db, userID); err == nil {
		return m, nil
	} else if !errors.Is(err, ErrNoModel) {
		return nil, err
	}
	all, err := syssettings.All(db.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("cannot read model settings")
	}
	var a syssettings.AgentSettings
	b, err := json.Marshal(all[syssettings.KeyAgent])
	if err != nil || json.Unmarshal(b, &a) != nil {
		return nil, ErrInvalidModel
	}
	if a.APIKey != "" && a.BaseURL != "" {
		m := a.Model
		if m == "" {
			m = cfg.AgentModel
		}
		return &ResolvedModel{
			Source: "admin-settings", Name: m, Model: m,
			BaseURL: a.BaseURL, APIKey: a.APIKey,
		}, nil
	}
	if cfg.AgentAPIKey != "" && cfg.AgentBaseURL != "" {
		return &ResolvedModel{
			Source: "server-env", Name: cfg.AgentModel, Model: cfg.AgentModel,
			BaseURL: cfg.AgentBaseURL, APIKey: cfg.AgentAPIKey,
		}, nil
	}
	return nil, ErrNoModel
}

// NewChatModel 用 eino-ext 的 OpenAI 兼容组件构建模型
// （DeepSeek/OpenAI/SiliconFlow 等兼容端点通用）。
// sessionID 用于 OpenCode 网关按会话稳定路由（官方要求，缺失时请求会被拒绝）。
func NewChatModel(ctx context.Context, r *ResolvedModel, sessionID string, timeout time.Duration) (model.BaseChatModel, error) {
	cfg := &openai.ChatModelConfig{
		APIKey:  r.APIKey,
		BaseURL: strings.TrimRight(r.BaseURL, "/"),
		Model:   r.Model,
	}
	base, _ := http.DefaultTransport.(*http.Transport)
	if base != nil {
		base = base.Clone()
	} else {
		base = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	if timeout > 0 {
		base.ResponseHeaderTimeout = timeout
	}
	var rt http.RoundTripper = base
	if sessionID != "" && isOpenCodeGateway(r.BaseURL) {
		rt = openCodeSessionTransport{base: rt, sessionID: sessionID}
	}
	cfg.HTTPClient = &http.Client{Transport: rt}
	return openai.NewChatModel(ctx, cfg)
}

func isOpenCodeGateway(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "opencode.ai" || strings.HasSuffix(host, ".opencode.ai")
}

type openCodeSessionTransport struct {
	base      http.RoundTripper
	sessionID string
}

func (t openCodeSessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if clone.Header.Get("x-opencode-session") == "" {
		clone.Header.Set("x-opencode-session", t.sessionID)
	}
	return t.base.RoundTrip(clone)
}
