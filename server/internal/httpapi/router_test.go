package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/0verL1nk/GraduateProject/server/internal/buildinfo"
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

// Health metadata is consumed by release verification without a configuration dependency.
func TestHealth(t *testing.T) {
	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/health")
	assert.Equal(t, 200, resp.StatusCode)

	assertBody(t, resp, `{"status":"healthy","version":"`+buildinfo.Version+`","commit":"`+buildinfo.Commit+`","buildTime":"`+buildinfo.Date+`","goVersion":"`+runtime.Version()+`"}`)
}

func TestHealthReflectsBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "1.2.3", "0123456789abcdef0123456789abcdef01234567", "2026-10-28T00:00:00Z"
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = oldVersion, oldCommit, oldDate
	})

	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/health")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assertBody(t, resp, `{"status":"healthy","version":"1.2.3","commit":"0123456789abcdef0123456789abcdef01234567","buildTime":"2026-10-28T00:00:00Z","goVersion":"`+runtime.Version()+`"}`)
}

func TestUnknownFrontendPathReturnsJSON404(t *testing.T) {
	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/writing")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assertBody(t, resp, `{"detail":"Not Found"}`)
}

// /api/v1 挂载点存在：其下未注册路径走统一 404 信封
func TestAPIV1Mount(t *testing.T) {
	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/api/v1/nope")
	assert.Equal(t, 404, resp.StatusCode)
	assertBody(t, resp, `{"detail":"Not Found"}`)
}

// 契约来源：backend/app/main.py CORS 配置 — 全开放 + expose Content-Disposition
func TestCORSHeaders(t *testing.T) {
	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	exposed := strings.ToLower(resp.Header.Get("Access-Control-Expose-Headers"))
	assert.Contains(t, exposed, "content-disposition")
	assert.Contains(t, exposed, "fileid")
	assert.Contains(t, exposed, "fontid")
	assert.Contains(t, exposed, "x-papersage-resource-revision")
}

func TestCORSAllowsLocalImportIdempotencyHeaders(t *testing.T) {
	srv := httptest.NewServer(New(testLogger()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/projects/", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-papersage-import-bundle-id,x-papersage-import-local-id")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	allowed := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers"))
	assert.Contains(t, allowed, "x-papersage-import-bundle-id")
	assert.Contains(t, allowed, "x-papersage-import-local-id")
	assert.Contains(t, allowed, "x-vercel-trusted-oidc-idp-token")
}
