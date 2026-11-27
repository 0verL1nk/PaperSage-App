package review

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Mount 注册检查域路由（挂在 /api/v1，均需登录）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("", authMw)
	g.GET("/projects/:project_id/review/options", s.optionsHandler)
}

func (s *Service) optionsHandler(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	out, err := s.Options(c.Request.Context(), auth.CurrentUser(c).ID, store.UUID{UUID: pid})
	switch {
	case errors.Is(err, ErrProjectNotFound):
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "项目不存在或无权访问"})
	case err != nil:
		slog.Error("review options failed", "project_id", pid.String(), "err", err)
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "读取项目结构失败，请稍后重试"})
	default:
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
	}
}
