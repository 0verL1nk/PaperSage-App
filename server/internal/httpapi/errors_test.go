package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：backend/app/exceptions.py — 错误响应逐字段复刻
func newTestAPI() *API {
	// nil dist：单测不挂 SPA（避免依赖构建产物），NoRoute 保持 404
	api := NewWithAssets(testLogger(), nil)
	probe := api.Group("/probe")
	probe.GET("/notfound", func(c *gin.Context) {
		WriteError(c.Writer, NotFound("Project"))
	})
	probe.GET("/unauthorized", func(c *gin.Context) {
		WriteError(c.Writer, Unauthorized("bad token"))
	})
	probe.GET("/forbidden", func(c *gin.Context) {
		WriteError(c.Writer, Forbidden("Forbidden"))
	})
	probe.GET("/badrequest", func(c *gin.Context) {
		WriteError(c.Writer, BadRequest("Bad request"))
	})
	probe.GET("/validation", func(c *gin.Context) {
		WriteError(c.Writer, ErrValidation)
	})
	probe.GET("/boom", func(c *gin.Context) {
		panic("unexpected")
	})
	probe.GET("/ok", func(c *gin.Context) {
		_ = WriteEnvelope(c.Writer, http.StatusOK, Success("fine", ""))
	})
	return api
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestAPIErrorMapping(t *testing.T) {
	cases := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/probe/notfound", 404, `{"code":404,"data":null,"info":"Project not found"}`},
		{"/probe/unauthorized", 401, `{"code":401,"data":null,"info":"bad token"}`},
		{"/probe/forbidden", 403, `{"code":403,"data":null,"info":"Forbidden"}`},
		{"/probe/badrequest", 400, `{"code":400,"data":null,"info":"Bad request"}`},
		{"/probe/validation", 422, `{"code":422,"data":null,"info":"请求参数验证失败，请检查输入"}`},
	}
	srv := httptest.NewServer(newTestAPI())
	t.Cleanup(srv.Close)

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			resp := get(t, srv.URL+tc.path)
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			assertBody(t, resp, tc.wantBody)
		})
	}
}

// panic 兜底：与 Python generic_exception_handler 输出一致
func TestPanicRecovery(t *testing.T) {
	srv := httptest.NewServer(newTestAPI())
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/probe/boom")
	assert.Equal(t, 500, resp.StatusCode)
	assertBody(t, resp, `{"code":500,"data":null,"info":"Internal server error"}`)

	// 恢复后服务仍可用
	resp = get(t, srv.URL+"/probe/ok")
	assert.Equal(t, 200, resp.StatusCode)
}

// 未知错误类型 → 500 通用信封（不泄漏内部信息）
func TestUnknownErrorMapping(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, fmt.Errorf("db connection refused: secret-details"))

	assert.Equal(t, 500, rec.Code)
	assert.JSONEq(t, `{"code":500,"data":null,"info":"Internal server error"}`, rec.Body.String())
}

// 未匹配路由 → 404 信封 info "Not Found"（对齐 Starlette 抛 HTTPException(404) 走自定义 handler）
func TestNotFoundRoute(t *testing.T) {
	srv := httptest.NewServer(newTestAPI())
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/definitely/not/registered")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assertBody(t, resp, `{"detail":"Not Found"}`)
}

// 方法不匹配 → 405 信封（对齐 Starlette Method Not Allowed）
func TestMethodNotAllowed(t *testing.T) {
	srv := httptest.NewServer(newTestAPI())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/probe/ok", "application/json", nil) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, 405, resp.StatusCode)
	assertBody(t, resp, `{"detail":"Method Not Allowed"}`)
}

// 域错误辅助构造器语义（对应 Python AppException 子类）
func TestAPIErrorConstructors(t *testing.T) {
	assert.Equal(t, &APIError{Code: 404, Info: "Document not found"}, NotFound("Document"))
	assert.Equal(t, &APIError{Code: 401, Info: "Unauthorized"}, Unauthorized(""))
	assert.Equal(t, &APIError{Code: 403, Info: "nope"}, Forbidden("nope"))
	assert.Equal(t, &APIError{Code: 400, Info: "Bad request"}, BadRequest(""))

	var target *APIError
	require.True(t, errors.As(error(NotFound("X")), &target))
	assert.Equal(t, 404, target.Code)
}
