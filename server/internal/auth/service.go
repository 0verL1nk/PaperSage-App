package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/api/routes/auth.py。

// Service 承载认证域逻辑（构造函数注入，无全局状态）。
type Service struct {
	DB      *gorm.DB
	Cfg     *config.Settings
	Mailer  email.Sender
	limiter *rateLimiter
}

// NewService wires the auth domain.
// refresh token 注册表走 DB（持久化；曾走进程内存 cache，重启即丢登录态）。
func NewService(db *gorm.DB, cfg *config.Settings, mailer email.Sender) *Service {
	if mailer == nil {
		mailer = email.Noop{}
	}
	return &Service{DB: db, Cfg: cfg, Mailer: mailer, limiter: newRateLimiter()}
}

// TokenResponse 对应 schemas/user.py TokenResponse（camelCase + tokenType）。
type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
}

// UserResponse 对应 schemas/user.py UserResponse（id 为带横线 UUID）。
type UserResponse struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	Email       *string   `json:"email"`
	IsActive    bool      `json:"is_active"`
	IsSuperuser bool      `json:"is_superuser"`
}

func toUserResponse(u *store.User) UserResponse {
	return UserResponse{ID: u.ID.UUID, Username: u.Username, Email: u.Email, IsActive: u.IsActive, IsSuperuser: u.IsSuperuser}
}

func errBad(info string, code int) *httpapi.APIError {
	return &httpapi.APIError{Code: code, Info: info}
}

// POST /auth/register（表单）→ UserResponse
// 两种形态共存：email+password（学校邮箱注册制，新）与
// account+password（用户名注册，存量/e2e 兼容）。
func (s *Service) register(c *gin.Context) {
	if !s.limiter.allow("register:"+c.ClientIP(), 10) {
		httpapi.WriteError(c.Writer, errBad("Too many requests", http.StatusTooManyRequests))
		return
	}
	password := c.PostForm("password")
	if password == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation) // FastAPI Form 缺失 → 422
		return
	}
	account := c.PostForm("account")
	if account == "" {
		// 无 account 但有 email：邮箱注册制（新）
		if e := c.PostForm("email"); e != "" {
			s.registerEmailFlow(c, e, password)
			return
		}
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var email *string
	if e := c.PostForm("email"); e != "" {
		email = &e
	}

	var existing store.User
	if err := s.DB.Where("username = ?", account).First(&existing).Error; err == nil {
		httpapi.WriteError(c.Writer, errBad("Username already exists", http.StatusBadRequest))
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	user := store.User{Username: account, PasswordHash: hash, Email: email}
	if err := s.DB.Create(&user).Error; err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(toUserResponse(&user), ""))
}

// GET /auth/me — 当前用户信息（登录态探测 + 前端条件渲染依据）
func (s *Service) me(c *gin.Context) {
	u := CurrentUser(c)
	if u == nil {
		httpapi.WriteError(c.Writer, errBad("Invalid token", http.StatusUnauthorized))
		return
	}
	resp := gin.H{
		"username":      u.Username,
		"isSuperuser":   u.IsSuperuser,
		"email":         nil,
		"emailVerified": u.EmailVerified,
	}
	if u.Email != nil {
		resp["email"] = *u.Email
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(resp, ""))
}

// POST /auth/login（表单）→ TokenResponse；refresh token 写入缓存
func (s *Service) login(c *gin.Context) {
	account := c.PostForm("account")
	password := c.PostForm("password")
	if account == "" || password == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	// 限速：IP 与账号双维度（校园出口 NAT 共享 → IP 阈值放宽）
	if !s.limiter.allow("login-ip:"+c.ClientIP(), 30) || !s.limiter.allow("login-acct:"+account, rateMaxLogin) {
		httpapi.WriteError(c.Writer, errBad("Too many attempts, try later", http.StatusTooManyRequests))
		return
	}
	// account 兼容 username 或 email（域名规范化后匹配）
	lookup := account
	if at := strings.LastIndex(account, "@"); at > 0 {
		lookup = account[:at] + "@" + strings.ToLower(account[at+1:])
	}

	var user store.User
	err := s.DB.Where("username = ? OR email = ?", account, lookup).First(&user).Error
	if err != nil || !user.IsActive || !VerifyPassword(password, user.PasswordHash) {
		// 三种失败统一 401 "Invalid credentials"（不泄漏用户存在性）
		httpapi.WriteError(c.Writer, errBad("Invalid credentials", http.StatusUnauthorized))
		return
	}
	// 邮箱注册用户未验证 → 403（需要正确密码才走到这，不构成无凭据枚举；
	// account 注册附带 email 的存量用户不受此门槛约束）
	if user.IsEmailRegistered && !user.EmailVerified {
		httpapi.WriteError(c.Writer, errBad("Please verify your email before logging in", http.StatusForbidden))
		return
	}

	s.maybePromoteSuperuser(c.Request.Context(), &user)
	sub := user.ID.String()
	access, _ := CreateAccessToken(s.Cfg.SecretKey, sub, s.Cfg.AccessTokenExpireMinutes)
	refresh, _ := CreateRefreshToken(s.Cfg.SecretKey, sub, s.Cfg.RefreshTokenExpireDays)
	s.storeRefresh(c.Request.Context(), refresh, user.ID, s.refreshTTL())

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(TokenResponse{
		AccessToken: access, RefreshToken: refresh, TokenType: "bearer",
	}, ""))
}

// maybePromoteSuperuser：SUPERUSER_EMAILS env（逗号分隔）命中即幂等
// 提升——部署者的 bootstrap 管理通道，不属于代码硬编码。
func (s *Service) maybePromoteSuperuser(ctx context.Context, user *store.User) {
	list := strings.Split(s.Cfg.SuperuserEmails, ",")
	email := ""
	if user.Email != nil {
		email = normalizeEmail(*user.Email)
	}
	for _, e := range list {
		e = strings.TrimSpace(e)
		if e == "" || email == "" || normalizeEmail(e) != email {
			continue
		}
		if !user.IsSuperuser {
			_ = s.DB.WithContext(ctx).Model(user).Update("is_superuser", true).Error
			user.IsSuperuser = true
		}
		return
	}
}

// POST /auth/refresh（JSON {refresh_token}）→ 新 token 对
// 注意对齐：Python 只写入新 refresh、不吊销旧的（测试锁定该行为）。
func (s *Service) refresh(c *gin.Context) {
	ctx := c.Request.Context()
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	claims, err := VerifyToken(req.RefreshToken, s.Cfg.SecretKey, "refresh")
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Invalid or expired refresh token", http.StatusUnauthorized))
		return
	}
	if claims.Subject == "" {
		httpapi.WriteError(c.Writer, errBad("Invalid refresh token", http.StatusUnauthorized))
		return
	}
	// 注册表校验（DB 持久化）：行存在、未吊销、未过期
	row := store.RefreshToken{TokenHash: tokenHash(req.RefreshToken)}
	err = s.DB.WithContext(ctx).Where("token_hash = ?", row.TokenHash).First(&row).Error
	if err != nil || row.RevokedAt != nil || time.Now().UTC().After(row.ExpiresAt) {
		httpapi.WriteError(c.Writer, errBad("Refresh token has been revoked", http.StatusUnauthorized))
		return
	}
	if row.UserID.String() != claims.Subject {
		// 改进：Python 不校验注册值与 token sub 的一致性（坏设计不迁移）
		httpapi.WriteError(c.Writer, errBad("Invalid or expired refresh token", http.StatusUnauthorized))
		return
	}

	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		// Python: uuid.UUID(user_id) 未捕获 → 500
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	var user store.User
	if err := s.DB.Where("id = ?", store.UUID{UUID: uid}).First(&user).Error; err != nil || !user.IsActive {
		httpapi.WriteError(c.Writer, errBad("User not found or inactive", http.StatusUnauthorized))
		return
	}

	sub := user.ID.String()
	access, _ := CreateAccessToken(s.Cfg.SecretKey, sub, s.Cfg.AccessTokenExpireMinutes)
	newRefresh, _ := CreateRefreshToken(s.Cfg.SecretKey, sub, s.Cfg.RefreshTokenExpireDays)
	// 真正的 refresh token rotation——签发新 token 时吊销旧行（DB）
	now := time.Now().UTC()
	_ = s.DB.WithContext(ctx).Model(&store.RefreshToken{}).
		Where("token_hash = ?", tokenHash(req.RefreshToken)).
		Update("revoked_at", now).Error
	s.storeRefresh(ctx, newRefresh, user.ID, s.refreshTTL())

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(TokenResponse{
		AccessToken: access, RefreshToken: newRefresh, TokenType: "bearer",
	}, ""))
}

// POST /auth/logout（JSON {refresh_token}）→ 恒 200，吊销 refresh
func (s *Service) logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	now := time.Now().UTC()
	_ = s.DB.WithContext(c.Request.Context()).Model(&store.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash(req.RefreshToken)).
		Update("revoked_at", now).Error
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(nil, ""))
}

// storeRefresh 写注册表行（FirstOrCreate：同 hash 重复写入幂等）
func (s *Service) storeRefresh(ctx context.Context, token string, uid store.UUID, ttl time.Duration) {
	row := store.RefreshToken{
		TokenHash: tokenHash(token),
		UserID:    uid,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	_ = s.DB.WithContext(ctx).Where("token_hash = ?", row.TokenHash).FirstOrCreate(&row).Error
}

// tokenHash：SHA-256 hex。token 是 JWE 密文（随机 IV），hash 不可逆推；
// 库泄露时不暴露可用凭据。
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) refreshTTL() time.Duration {
	return time.Duration(s.Cfg.RefreshTokenExpireDays*24) * time.Hour
}
