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
		h := c.GetHeader("Authorization")
		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Not authenticated"})
			c.Abort()
			return
		}
		claims, err := VerifyToken(parts[1], s.Cfg.SecretKey, "access")
		if err != nil || claims.Subject == "" {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Invalid token"})
			c.Abort()
			return
		}
		uid, err := uuid.Parse(claims.Subject)
		if err != nil {
			// 坏设计修正：Python 未捕获 uuid.UUID 异常返回 500 → 401
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "Invalid token"})
			c.Abort()
			return
		}
		var user store.User
		if err := s.DB.WithContext(c.Request.Context()).Where("id = ?", store.UUID{UUID: uid}).First(&user).Error; err != nil || !user.IsActive {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusUnauthorized, Info: "User not found or inactive"})
			c.Abort()
			return
		}
		c.Set(ctxUserKey, &user)
		c.Next()
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
