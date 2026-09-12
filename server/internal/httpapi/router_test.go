package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func assertBody(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(b))
}

// 契约来源：backend/app/main.py — GET /health 返回裸 JSON（不走信封）
func TestHealth(t *testing.T) {
	srv := httptest.NewServer(NewWithAssets(testLogger(), nil))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/health")
	assert.Equal(t, 200, resp.StatusCode)

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"status":"healthy","version":"0.1.0"}`, string(b))
}

// /api/v1 挂载点存在：其下未注册路径走统一 404 信封
func TestAPIV1Mount(t *testing.T) {
	srv := httptest.NewServer(NewWithAssets(testLogger(), nil))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/api/v1/nope")
	assert.Equal(t, 404, resp.StatusCode)
	assertBody(t, resp, `{"detail":"Not Found"}`)
}

// 契约来源：backend/app/main.py CORS 配置 — 全开放 + expose Content-Disposition
func TestCORSHeaders(t *testing.T) {
	srv := httptest.NewServer(NewWithAssets(testLogger(), nil))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Content-Disposition", resp.Header.Get("Access-Control-Expose-Headers"))
}
