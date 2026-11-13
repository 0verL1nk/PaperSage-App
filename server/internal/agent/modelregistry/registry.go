// Package modelregistry 动态维护模型能力目录。
//
// 数据来自 models.dev（MIT）的 /api.json，运行时后台刷新并缓存在内存里，
// 不在请求路径上抓取，也不把上万条模型写死进仓库。解析失败或尚未刷新时
// 一律返回“未知”（nil），由用户设置里的 model_metadata 覆盖；自定义
// provider/model 本来就无法从公共目录获得，必须在设置页手填。
package modelregistry

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Metadata 三态语义：nil 表示未知，false 表示明确不支持。
type Metadata struct {
	ContextWindow   *int  `json:"context_window,omitempty"`
	MaxOutputTokens *int  `json:"max_output_tokens,omitempty"`
	SupportsVision  *bool `json:"supports_vision,omitempty"`
	SupportsTools   *bool `json:"supports_tools,omitempty"`
}

// Registry 是进程内目录；零值可用。
type Registry struct {
	mu        sync.RWMutex
	models    map[string]Metadata
	byBase    map[string]Metadata
	source    string
	fetchedAt time.Time
}

// Default 是 agent 解析模型时使用的共享目录，由 bootstrap 启动刷新。
var Default = &Registry{}

// Lookup 按模型 ID 精确匹配；找不到时尝试去掉 provider 前缀再匹配一次。
func (r *Registry) Lookup(model string) (Metadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := strings.TrimSpace(model)
	if id == "" || len(r.models) == 0 {
		return Metadata{}, false
	}
	if m, ok := r.models[id]; ok {
		return m, true
	}
	if i := strings.LastIndex(id, "/"); i >= 0 && i+1 < len(id) {
		if m, ok := r.models[id[i+1:]]; ok {
			return m, true
		}
	}
	if m, ok := r.byBase[id]; ok {
		return m, true
	}
	return Metadata{}, false
}

// Status 返回当前快照的来源、模型数与刷新时间（设置页展示用）。
func (r *Registry) Status() (source string, count int, fetchedAt time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.source, len(r.models), r.fetchedAt
}

// Start 后台刷新目录：先立即抓一次，之后按 interval 刷新。
// 失败只记日志，保留上一份快照；url 为空时禁用。
func (r *Registry) Start(ctx context.Context, url string, interval time.Duration) {
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	refresh := func() {
		if err := r.Refresh(ctx, url); err != nil {
			slog.Warn("model registry refresh failed", "url", url, "err", err)
		}
	}
	go func() {
		refresh()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

type sourceModel struct {
	ID         string `json:"id"`
	ToolCall   *bool  `json:"tool_call"`
	Attachment *bool  `json:"attachment"`
	Modalities *struct {
		Input []string `json:"input"`
	} `json:"modalities"`
	Limit *struct {
		Context *int `json:"context"`
		Output  *int `json:"output"`
	} `json:"limit"`
}

// httpClient 独立于 http.DefaultClient：目录下载不能无限期挂住后台 goroutine。
var httpClient = &http.Client{Timeout: 60 * time.Second}

// Refresh 抓取并替换快照；只有完整解析成功才替换，避免半份数据。
func (r *Registry) Refresh(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode}
	}
	var providers map[string]struct {
		Models map[string]sourceModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&providers); err != nil {
		return err
	}
	models := make(map[string]Metadata, 9000)
	for _, p := range providers {
		for id, m := range p.Models {
			md, ok := convert(m)
			if !ok {
				continue
			}
			if prev, exists := models[id]; exists {
				models[id] = merge(prev, md)
			} else {
				models[id] = md
			}
		}
	}
	byBase := make(map[string]Metadata, len(models))
	ambiguous := make(map[string]bool)
	for id, md := range models {
		i := strings.LastIndex(id, "/")
		if i < 0 || i+1 >= len(id) {
			continue
		}
		base := id[i+1:]
		if _, exists := byBase[base]; exists {
			ambiguous[base] = true
			continue
		}
		byBase[base] = md
	}
	for base := range ambiguous {
		delete(byBase, base)
	}
	r.mu.Lock()
	r.models = models
	r.byBase = byBase
	r.source = "models.dev api.json"
	r.fetchedAt = time.Now().UTC()
	r.mu.Unlock()
	slog.Info("model registry refreshed", "models", len(models))
	return nil
}

func convert(m sourceModel) (Metadata, bool) {
	var md Metadata
	if m.Limit != nil {
		md.ContextWindow = positive(m.Limit.Context)
		md.MaxOutputTokens = positive(m.Limit.Output)
	}
	if m.Modalities != nil {
		vision := false
		for _, in := range m.Modalities.Input {
			if in == "image" {
				vision = true
				break
			}
		}
		md.SupportsVision = &vision
	} else {
		// 没有 modalities 时退回 attachment：能收附件通常意味着能读图。
		md.SupportsVision = m.Attachment
	}
	md.SupportsTools = m.ToolCall
	return md, md.ContextWindow != nil || md.MaxOutputTokens != nil ||
		md.SupportsVision != nil || md.SupportsTools != nil
}

func positive(v *int) *int {
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

// merge 处理不同 provider 下同名模型：上下文取较大值，避免过度裁剪；
// 布尔能力取首个非 nil 值。用户仍可用 model_metadata 精确覆盖。
func merge(a, b Metadata) Metadata {
	if a.ContextWindow == nil || (b.ContextWindow != nil && *b.ContextWindow > *a.ContextWindow) {
		a.ContextWindow = b.ContextWindow
	}
	if a.MaxOutputTokens == nil || (b.MaxOutputTokens != nil && *b.MaxOutputTokens > *a.MaxOutputTokens) {
		a.MaxOutputTokens = b.MaxOutputTokens
	}
	if a.SupportsVision == nil {
		a.SupportsVision = b.SupportsVision
	}
	if a.SupportsTools == nil {
		a.SupportsTools = b.SupportsTools
	}
	return a
}

type httpError struct{ status int }

func (e *httpError) Error() string {
	return "model registry returned HTTP " + http.StatusText(e.status)
}
