package httpapi

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约：design D7 / single-binary-delivery spec
func newSPAServer(t *testing.T) *httptest.Server {
	t.Helper()
	dist := fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<html>SPA</html>")},
		"assets/app.abc123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
		"favicon.ico":          &fstest.MapFile{Data: []byte("ICON")},
	}
	srv := httptest.NewServer(NewWithAssets(testLogger(), dist))
	t.Cleanup(srv.Close)
	return srv
}

func TestSPAServing(t *testing.T) {
	srv := newSPAServer(t)

	// 根路径 → index.html + no-cache
	resp := get(t, srv.URL+"/")
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	// 深链接刷新 → SPA fallback（history 路由不 404）
	resp = get(t, srv.URL+"/auth")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))

	// 命中 assets → immutable 长缓存 + 正确 mime
	resp = get(t, srv.URL+"/assets/app.abc123.js")
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
	assert.Contains(t, resp.Header.Get("Content-Type"), "javascript")

	// 其他静态文件 no-cache
	resp = get(t, srv.URL+"/favicon.ico")
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	// API 前缀未匹配 → 404 信封（不受 SPA fallback 影响）
	resp = get(t, srv.URL+"/api/v1/ghost")
	assert.Equal(t, 404, resp.StatusCode)
	assertBody(t, resp, `{"detail":"Not Found"}`)

	// 带扩展名但不存在 → 404（不回退 index）
	resp = get(t, srv.URL+"/missing.png")
	assert.Equal(t, 404, resp.StatusCode)

	// /health 仍工作
	resp = get(t, srv.URL+"/health")
	assert.Equal(t, 200, resp.StatusCode)
}
