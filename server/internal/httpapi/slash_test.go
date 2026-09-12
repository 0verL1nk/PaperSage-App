package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约背景：CI/生产的前端可能跨域直连后端（VITE_API_BASE_URL）。
// gin 默认 RedirectTrailingSlash 对 /folders → /folders/ 发 307，
// 且重定向响应绕过中间件链、不带 CORS 头——跨域浏览器直接判网络
// 错误（"Network request failed"，CI e2e 实际踩中）。修复语义：
// 斜杠变体在服务端直接重路由服务，不再依赖 307。
func TestSlashVariantRerouted(t *testing.T) {
	api := New(slog.New(slog.DiscardHandler))
	v1 := api.Group(APIVersion)

	var hits int
	withSlash := v1.Group("/folders")
	withSlash.POST("/", func(c *gin.Context) { hits++; _ = WriteEnvelope(c.Writer, 200, Success(gin.H{"ok": true}, "")) })
	var hitsNS int
	v1.POST("/noslash", func(c *gin.Context) { hitsNS++; _ = WriteEnvelope(c.Writer, 200, Success(gin.H{"ok": true}, "")) })

	// 注册带斜杠，请求不带斜杠 → 直接 200（不是 307）
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/folders", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173") // CORS 头只对带 Origin 的请求设置
	api.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "无斜杠请求应直接服务而非重定向")
	assert.Equal(t, 1, hits)
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Origin"), "*", "重路由走完整中间件链，CORS 头必须在场")
	// 响应体必须恰好是一个可解析 JSON：HandleContext 重路由曾把 handler
	// 跑两遍（外层 c.Next() 继续进入路由链尾部），200 信封后拼接第二个
	// 422 信封，浏览器端 JSON.parse 直接失败
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed), "响应体必须是单一合法 JSON：%s", w.Body.String())
	assert.Equal(t, float64(200), parsed["code"])

	// 反向：注册不带斜杠，请求带斜杠 → 200
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/noslash/", nil)
	api.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, 1, hitsNS)

	// 真正不存在的路径仍然 404（重写只尝试一次，无振荡）
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/definitely-missing", nil)
	api.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code)
	assert.Contains(t, w3.Body.String(), `"detail"`, "未命中路由保持 Starlette 风格 404")
}
