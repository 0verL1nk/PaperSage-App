package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 分类校验矩阵：对齐各 Pydantic schema 约束
func TestNormalizeCategoryMatrix(t *testing.T) {
	cases := []struct {
		name    string
		cat     string
		in      map[string]any
		wantErr bool
		check   func(t *testing.T, out map[string]any)
	}{
		{"ui 合法", "ui", map[string]any{"theme": "dark"}, false, func(t *testing.T, out map[string]any) {
			assert.Equal(t, "dark", out["theme"])
			assert.Equal(t, "zh-CN", out["language"])
		}},
		{"ui theme 非法", "ui", map[string]any{"theme": "purple"}, true, nil},
		{"ui language 非法", "ui", map[string]any{"language": "zh_cn"}, true, nil},
		{"ui sidebar 类型错", "ui", map[string]any{"sidebar_collapsed": "yes"}, true, nil},
		{"editor 合法", "editor", map[string]any{"font_size": 20, "tab_size": 4}, false, func(t *testing.T, out map[string]any) {
			assert.Equal(t, 20, out["font_size"])
			assert.Equal(t, 4, out["tab_size"])
			assert.Equal(t, true, out["auto_save"])
		}},
		{"editor 非整数", "editor", map[string]any{"font_size": 14.5}, true, nil},
		{"editor 超上限", "editor", map[string]any{"font_size": 40}, true, nil},
		{"editor tab 下限", "editor", map[string]any{"tab_size": 0}, true, nil},
		{"editor interval 下限", "editor", map[string]any{"auto_save_interval": 1}, true, nil},
		{"editor word_wrap 类型", "editor", map[string]any{"word_wrap": 1}, true, nil},
		{"agent 合法", "agent", map[string]any{"temperature": 1.5, "writing_style": "formal"}, false, func(t *testing.T, out map[string]any) {
			assert.Equal(t, 1.5, out["temperature"])
			assert.Equal(t, "formal", out["writing_style"])
		}},
		{"agent temperature 超", "agent", map[string]any{"temperature": 2.5}, true, nil},
		{"agent temperature 类型", "agent", map[string]any{"temperature": "high"}, true, nil},
		{"agent style 空", "agent", map[string]any{"writing_style": ""}, true, nil},
		{"agent providers 非列表", "agent", map[string]any{"providers": "x"}, true, nil},
		{"agent provider 元素非对象", "agent", map[string]any{"providers": []any{"x"}}, true, nil},
		{"agent provider type 非法", "agent", map[string]any{"providers": []any{map[string]any{"type": "x"}}}, true, nil},
		{"agent provider name 超长", "agent", map[string]any{"providers": []any{map[string]any{"name": string([]byte{0x61}) + pad101(t)}}}, true, nil},
		{"agent provider base_url 超长", "agent", map[string]any{"providers": []any{map[string]any{"base_url": pad501(t)}}}, true, nil},
		{"agent provider api_key 超长", "agent", map[string]any{"providers": []any{map[string]any{"api_key": pad501(t)}}}, true, nil},
		{"agent provider models 非列表", "agent", map[string]any{"providers": []any{map[string]any{"models": "m"}}}, true, nil},
		{"agent provider id 超长", "agent", map[string]any{"providers": []any{map[string]any{"id": pad65(t)}}}, true, nil},
		{"agent memory_enabled 类型", "agent", map[string]any{"memory_enabled": "on"}, true, nil},
		{"未知分类", "other", map[string]any{}, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := NormalizeCategory(tc.cat, tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, out)
			}
		})
	}
}

func pad101(t *testing.T) string { t.Helper(); return repeat(101) }
func pad501(t *testing.T) string { t.Helper(); return repeat(501) }
func pad65(t *testing.T) string  { t.Helper(); return repeat(65) }

func repeat(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestNormalizeRejectsUnknownActiveProvider(t *testing.T) {
	_, err := NormalizeCategory("agent", map[string]any{
		"active_provider_id": "missing",
		"providers":          []any{map[string]any{"id": "p", "models": []any{"A"}, "active_model": "A"}},
	})
	require.Error(t, err)
}

func TestNormalizeRejectsInvalidModelSelection(t *testing.T) {
	for _, provider := range []map[string]any{
		{"models": []any{map[string]any{"x": 1}}},
		{"models": []any{"A", 42}},
		{"models": []any{" "}},
		{"models": []any{"A"}, "active_model": "B"},
		{"models": []any{"A"}, "active_model": ""},
	} {
		_, err := NormalizeCategory("agent", map[string]any{"providers": []any{provider}})
		require.Error(t, err)
	}
}

func TestNormalizeLegacyProviderKeepsDefaultModel(t *testing.T) {
	out, err := NormalizeCategory("agent", map[string]any{"providers": []any{map[string]any{"id": "p", "models": []any{"A", "B"}}}})
	require.NoError(t, err)
	provider := out["providers"].([]any)[0].(map[string]any)
	assert.Equal(t, "B", provider["active_model"])
}

// Error() 接口实现
func TestErrorMessages(t *testing.T) {
	assert.Equal(t, "boom", (&ValidationError{Msg: "boom"}).Error())
}

// ValidCategories 导出清单与实现一致（防漂移）
func TestValidCategoriesMatch(t *testing.T) {
	for _, c := range ValidCategories {
		assert.True(t, isValidCategory(c))
	}
	assert.False(t, isValidCategory("nope"))
}

func TestNormalizeProviderModelMetadata(t *testing.T) {
	base := func(meta any) map[string]any {
		return map[string]any{
			"providers": []any{map[string]any{
				"id": "p", "name": "P", "base_url": "https://example.com", "api_key": "k",
				"models": []any{"m"}, "active_model": "m", "model_metadata": meta,
			}},
		}
	}
	out, err := NormalizeCategory("agent", base(map[string]any{
		"m": map[string]any{"context_window": float64(128000), "supports_vision": true, "supports_tools": false},
	}))
	require.NoError(t, err)
	provider := out["providers"].([]any)[0].(map[string]any)
	meta := provider["model_metadata"].(map[string]any)["m"].(map[string]any)
	require.Equal(t, 128000, meta["context_window"])
	require.Equal(t, true, meta["supports_vision"])
	require.Equal(t, false, meta["supports_tools"])

	for name, meta := range map[string]any{
		"not an object":      "x",
		"unknown model":      map[string]any{"other": map[string]any{"context_window": float64(1)}},
		"negative window":    map[string]any{"m": map[string]any{"context_window": float64(-1)}},
		"non-integer window": map[string]any{"m": map[string]any{"context_window": 1.5}},
		"bad vision flag":    map[string]any{"m": map[string]any{"supports_vision": "yes"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeCategory("agent", base(meta))
			require.Error(t, err)
		})
	}
}
