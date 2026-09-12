package httpapi

import (
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/web"
)

// SPA 静态服务（design D7）：
//   1. /api/* 与 /health → API/404 信封（不做 fallback）
//   2. embed FS 精确命中 → 服务；/assets/* immutable 长缓存
//   3. 无扩展名路径 → index.html（no-cache，SPA 深链接回退）
//   4. 其余 → 404 信封

// NewWithAssets 以注入的静态文件系统构建 API（测试用 fstest.MapFS）。
func NewWithAssets(logger *slog.Logger, dist fs.FS) *API {
	api := newEngine(logger)
	api.mountSPA(dist)
	return api
}

// mountSPA 注册静态回退（由 New 里的 NoRoute 在斜杠重写之后调用）。
func (a *API) mountSPA(dist fs.FS) {
	if dist == nil {
		return
	}
	a.spaFallback = func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/health" || strings.HasPrefix(p, "/api/") {
			writeDetail(c, http.StatusNotFound, "Not Found")
			return
		}
		clean := strings.TrimPrefix(path.Clean("/"+p), "/")

		if clean != "" {
			if data, err := fs.ReadFile(dist, clean); err == nil {
				serveStatic(c, clean, data)
				return
			}
		}
		// 无扩展名 → SPA fallback；带扩展名但缺失 → 404
		if strings.Contains(path.Base(clean), ".") && clean != "" {
			writeDetail(c, http.StatusNotFound, "Not Found")
			return
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			writeDetail(c, http.StatusNotFound, "Not Found")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(index)
	}
}

func serveStatic(c *gin.Context, name string, data []byte) {
	if strings.HasPrefix(name, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Header("Content-Type", ct)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(data)
}

// 默认 dist：编译进来的前端产物（本地开发只有占位文件）。
func defaultDist() fs.FS {
	dist, err := web.Dist()
	if err != nil {
		return nil
	}
	return dist
}
