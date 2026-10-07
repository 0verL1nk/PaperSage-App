package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/api/dependencies.py get_current_user。
// 401/500 文案逐条对齐：
//   - 无 Authorization / 非 Bearer → 401 "Not authenticated"
//   - 验签失败 / sub 缺失 → 401 "Invalid token"
//   - sub 非 UUID → 401（坏设计修正：Python 未捕获异常返回 500）
//   - 用户不存在或 inactive → 401 "User not found or inactive"

const ctxUserKey = "papersage.user"

// AuthRequired 解析 Bearer access token 并注入当前用户。
func (s *Service) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.authCheck(c) {
			c.Next()
		}
	}
}

// authCheck 校验并注入当前用户；失败时已写错误并 Abort。
// 不含 c.Next()——ChainMiddleware 直调它时不会误放行后续链条
// （曾因此 200/403 双信封：内层 c.Next() 把 handler 先跑了一遍）。
func (s *Service) authCheck(c *gin.Context) bool {
	{
		h := c.GetHeader("Authorization")
		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Not authenticated"})
			c.Abort()
			return false
		}
		claims, err := VerifyToken(parts[1], s.Cfg.SecretKey, "access")
		if err != nil || claims.Subject == "" {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Invalid token"})
			c.Abort()
			return false
		}
		uid, err := uuid.Parse(claims.Subject)
		if err != nil {
			// 坏设计修正：Python 未捕获 uuid.UUID 异常返回 500 → 401
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Invalid token"})
			c.Abort()
			return false
		}
		var user store.User
		if err := s.DB.WithContext(c.Request.Context()).Where("id = ?", store.UUID{UUID: uid}).First(&user).Error; err != nil || !user.IsActive {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "User not found or inactive"})
			c.Abort()
			return false
		}
		if claims.AuthVersion != user.AuthVersion {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Invalid token"})
			c.Abort()
			return false
		}
		c.Set(ctxUserKey, &user)
		return true
	}
}

// CurrentUser 从请求上下文取认证用户（AuthRequired 之后可用）。
func CurrentUser(c *gin.Context) *store.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*store.User)
	return u
}

// ChainMiddleware 把 AuthRequired 与后续守卫串成一个中间件
// （管理路由用：登录 + superuser）。
func (s *Service) ChainMiddleware(next ...gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.authCheck(c) {
			return
		}
		for _, h := range next {
			h(c)
			if c.IsAborted() {
				return
			}
		}
	}
}

// AdminRequired superuser 专属中间件（叠加在 AuthRequired 之后）。
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
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
