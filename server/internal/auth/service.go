package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/api/routes/auth.py。

// Service 承载认证域逻辑（构造函数注入，无全局状态）。
type Service struct {
	DB  *gorm.DB
	Cfg *config.Settings
}

// NewService wires the auth domain.
// refresh token 注册表走 DB（持久化；曾走进程内存 cache，重启即丢登录态）。
func NewService(db *gorm.DB, cfg *config.Settings) *Service {
	return &Service{DB: db, Cfg: cfg}
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
func (s *Service) register(c *gin.Context) {
	account := c.PostForm("account")
	password := c.PostForm("password")
	if account == "" || password == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation) // FastAPI Form 缺失 → 422
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

// POST /auth/login（表单）→ TokenResponse；refresh token 写入缓存
func (s *Service) login(c *gin.Context) {
	account := c.PostForm("account")
	password := c.PostForm("password")
	if account == "" || password == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	var user store.User
	err := s.DB.Where("username = ?", account).First(&user).Error
	if err != nil || !user.IsActive || !VerifyPassword(password, user.PasswordHash) {
		// 三种失败统一 401 "Invalid credentials"（不泄漏用户存在性）
		httpapi.WriteError(c.Writer, errBad("Invalid credentials", http.StatusUnauthorized))
		return
	}

	sub := user.ID.String()
	access, _ := CreateAccessToken(s.Cfg.SecretKey, sub, s.Cfg.AccessTokenExpireMinutes)
	refresh, _ := CreateRefreshToken(s.Cfg.SecretKey, sub, s.Cfg.RefreshTokenExpireDays)
	s.storeRefresh(c.Request.Context(), refresh, user.ID, s.refreshTTL())

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(TokenResponse{
		AccessToken: access, RefreshToken: refresh, TokenType: "bearer",
	}, ""))
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
	if err != nil || row.RevokedAt != nil || time.Now().After(row.ExpiresAt) {
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
	now := time.Now()
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
	now := time.Now()
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
		ExpiresAt: time.Now().Add(ttl),
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
