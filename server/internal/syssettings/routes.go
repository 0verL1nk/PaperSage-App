// Package syssettings — HTTP 层：admin 读写系统设置。
package syssettings

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
)

// AdminRequired：superuser 专属中间件（叠加在 AuthRequired 之后）。
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || !u.IsSuperuser {
			httpapi.WriteError(c.Writer, &httpapi.APIError{
				Code: http.StatusForbidden, Info: "Admin access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

type Service struct{ DB *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) get(c *gin.Context) {
	all, err := All(s.DB)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(all, ""))
}

func (s *Service) put(c *gin.Context) {
	var kv map[string]any
	if err := c.ShouldBindJSON(&kv); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if err := Put(s.DB, kv); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	all, _ := All(s.DB)
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(all, ""))
}

// Mount 挂载 admin 设置路由。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("/admin", authMw, AdminRequired())
	g.GET("/settings", s.get)
	g.PUT("/settings", s.put)
}
