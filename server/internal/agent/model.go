// Package agent — AI 写作助手域（Eino ADK，ReAct 工具循环）。
//
// 模型解析链：用户设置 agent.providers[active_provider_id]（BYO key）
// → 服务端 env（AGENT_API_KEY/BASE_URL/MODEL）兜底；都没有则报错，
// 前端引导到设置页。模型实例按请求构建，无全局态。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// ProviderConfig 是 settings.agent.providers[] 的单条形状
// （与前端 AgentSettings/model-selector 契约一致）。
type ProviderConfig struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	BaseURL  string   `json:"base_url"`
	APIKey   string   `json:"api_key"`
	Models   []string `json:"models"`
	IsActive bool     `json:"is_active,omitempty"`
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

// ResolveModel 按链解析：user settings → env。
func ResolveModel(ctx context.Context, db *gorm.DB, userID interface{ String() string }) (*ResolvedModel, error) {
	// 1) 用户设置
	raw := db.WithContext(ctx).
		Table("user_settings").
		Select("settings").
		Where("user_id = ? AND category = ?", userID.String(), "agent").
		Row()
	var buf []byte
	if err := raw.Scan(&buf); err == nil && len(buf) > 0 {
		var s struct {
			Providers        []ProviderConfig `json:"providers"`
			ActiveProviderID string           `json:"active_provider_id"`
		}
		if json.Unmarshal(buf, &s) == nil {
			for _, p := range s.Providers {
				if p.APIKey != "" && p.BaseURL != "" && len(p.Models) > 0 &&
					(p.ID == s.ActiveProviderID || s.ActiveProviderID == "" || p.IsActive) {
					m := p.Models[len(p.Models)-1]
					if s.ActiveProviderID != "" && p.ID != s.ActiveProviderID {
						continue
					}
					return &ResolvedModel{
						Source: "user-settings", Name: p.Name, Model: m,
						BaseURL: p.BaseURL, APIKey: p.APIKey,
					}, nil
				}
			}
		}
	}
	return nil, ErrNoModel
}

// ResolveModelWithEnv 在用户配置未命中时落 env 兜底。
func ResolveModelWithEnv(ctx context.Context, db *gorm.DB, userID interface{ String() string }, cfg *config.Settings) (*ResolvedModel, error) {
	if m, err := ResolveModel(ctx, db, userID); err == nil {
		return m, nil
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
func NewChatModel(ctx context.Context, r *ResolvedModel) (model.BaseChatModel, error) {
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  r.APIKey,
		BaseURL: strings.TrimRight(r.BaseURL, "/"),
		Model:   r.Model,
	})
}
