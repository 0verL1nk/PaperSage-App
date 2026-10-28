package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"strings"

	cors "github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/buildinfo"
)

// APIVersion is the API prefix mounted under /api/v1 (matches FastAPI app).
const APIVersion = "/api/v1"

// API assembles the gin engine: middleware, error handling, health, /api/v1.
// 领域路由在后续任务组通过 api.Group(APIVersion) 挂载。
type API struct {
	*gin.Engine
}

// New builds the API engine.
func New(logger *slog.Logger) *API {
	return newEngine(logger)
}

// newEngine 组装中间件/错误处理/health。
func newEngine(logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	gin.SetMode(gin.ReleaseMode)

	e := gin.New()
	api := &API{Engine: e}
	e.Use(recoverMiddleware(logger))
	// gin-contrib/cors：对应 FastAPI allow_origins=*, expose Content-Disposition
	e.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders:    []string{"Authorization", "Content-Type", "Accept", "Origin", "X-Requested-With", "X-PaperSage-Import-Bundle-Id", "X-PaperSage-Import-Local-Id", "X-Vercel-Trusted-OIDC-IdP-Token"},
		ExposeHeaders:   []string{"Content-Disposition", "ETag", "Content-Length", "fileid", "fontid", "X-PaperSage-Resource-Revision"},
	}))

	// 未匹配路由/方法 → Starlette 实测形态：裸 {"detail": ...}（双跑验证，
	// FastAPI 的 HTTPException handler 对 router 404/405 不生效）
	e.HandleMethodNotAllowed = true
	// gin 的 RedirectTrailingSlash 发出的 307 绕过中间件链、不带 CORS 头，
	// 跨域浏览器（CI/生产前端直连）按网络错误处理；改为 NoRoute 里做
	// 斜杠变体重写并重路由（走完整中间件链，CORS 头在场）
	e.RedirectTrailingSlash = false
	e.NoRoute(func(c *gin.Context) {
		// 防振荡标记必须挂在 request context 上：HandleContext 会
		// c.reset() 清空 Context.Keys，gin 自己的键放不住
		if c.Request.Context().Value(ctxSlashTriedKey{}) == nil {
			alt := c.Request.URL.Path
			if strings.HasSuffix(alt, "/") && len(alt) > 1 {
				alt = strings.TrimSuffix(alt, "/")
			} else {
				alt += "/"
			}
			req := c.Request.WithContext(context.WithValue(c.Request.Context(), ctxSlashTriedKey{}, true))
			req.URL.Path = alt
			c.Request = req
			e.HandleContext(c)
			// HandleContext 与外层 NoRoute 共享同一 Context 的 handlers/index：
			// 路由链比 NoRoute 链长，闭包返回后外层 c.Next() 会继续跑进路由链
			// 尾部、把 handler 再执行一遍（body 已消费 → 二次 422 拼接在
			// 200 之后）。Abort 把 index 推到链尾，截断外层循环。
			c.Abort()
			return
		}
		c.Header("Cache-Control", "no-store")
		writeDetail(c, http.StatusNotFound, "Not Found")
	})
	e.NoMethod(func(c *gin.Context) {
		writeDetail(c, http.StatusMethodNotAllowed, "Method Not Allowed")
	})

	e.GET("/health", healthHandler)
	_ = e.Group(APIVersion) // 领域路由挂载点

	return api
}

// ctxSlashTriedKey 标记本请求已做过一次斜杠变体重写（防振荡）。
type ctxSlashTriedKey struct{}

// writeDetail 输出 Starlette 风格 {"detail": msg}。
func writeDetail(c *gin.Context, status int, msg string) {
	c.Header("Content-Type", "application/json")
	c.Status(status)
	_, _ = c.Writer.Write([]byte(`{"detail":"` + msg + `"}`))
}

func healthHandler(c *gin.Context) {
	body, err := json.Marshal(struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildTime string `json:"buildTime"`
		GoVersion string `json:"goVersion"`
	}{
		Status:    "healthy",
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		BuildTime: buildinfo.Date,
		GoVersion: runtime.Version(),
	})
	if err != nil {
		_ = WriteEnvelope(c.Writer, http.StatusInternalServerError, Failure(http.StatusInternalServerError, "Internal server error"))
		return
	}
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(body)
}

// recoverMiddleware 把 handler panic 转成与 Python generic_exception_handler
// 一致的信封输出（gin 自带 Recovery 是纯文本 500，不满足契约）。
func recoverMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				logger.Error("panic recovered", "error", rec, "method", c.Request.Method, "path", c.Request.URL.Path)
				_ = WriteEnvelope(c.Writer, http.StatusInternalServerError, Failure(http.StatusInternalServerError, "Internal server error"))
			}
		}()
		c.Next()
	}
}
