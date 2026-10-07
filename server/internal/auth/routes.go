package auth

import "github.com/gin-gonic/gin"

// Mount 注册认证域路由（挂在 /api/v1 组下，对应 FastAPI prefix="/auth"）。
func Mount(rg *gin.RouterGroup, s *Service) {
	rg.POST("/auth/register", s.register)
	rg.POST("/auth/login", s.login)
	rg.POST("/auth/refresh", s.refresh)
	rg.POST("/auth/logout", s.logout)
	rg.GET("/auth/me", s.AuthRequired(), s.me)
	rg.GET("/auth/verify-email", s.verifyEmail)
	rg.GET("/auth/email-domains", s.emailDomainsHandler)
	rg.POST("/auth/resend-verification", s.resendVerification)
	rg.POST("/auth/password-reset/request", s.requestPasswordReset)
	rg.POST("/auth/password-reset/confirm", s.confirmPasswordReset)
}
