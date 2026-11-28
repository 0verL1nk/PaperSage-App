package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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

// refreshGraceWindow：多标签页并发刷新的宽限窗口。窗口内被轮换的旧 token
// 平滑续签，窗口外视为凭证重放并整族吊销（对齐 GoTrue/Auth0 的 reuse interval）。
const refreshGraceWindow = 15 * time.Second

// Service 承载认证域逻辑（构造函数注入，无全局状态）。
type Service struct {
	DB      *gorm.DB
	Cfg     *config.Settings
	Mailer  email.Sender
	limiter *rateLimiter

	// 过期 refresh 行惰性清理节流（每进程每小时最多一次，不引入定时器）。
	cleanupMu   sync.Mutex
	lastCleanup time.Time
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
		"hasPassword":   hasPassword(u.PasswordHash),
	}
	if u.Email != nil {
		resp["email"] = *u.Email
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(resp, ""))
}

// POST /auth/login（表单）→ TokenResponse；一次登录新建一个 refresh token family。
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

	ctx := c.Request.Context()
	s.maybePromoteSuperuser(ctx, &user)
	sub := user.ID.String()
	access, _ := CreateVersionedAccessToken(s.Cfg.SecretKey, sub, user.AuthVersion, s.Cfg.AccessTokenExpireMinutes)
	refresh, _ := CreateVersionedRefreshToken(s.Cfg.SecretKey, sub, user.AuthVersion, s.Cfg.RefreshTokenExpireDays)
	s.storeRefresh(ctx, refresh, user.ID, s.refreshTTL(), newRefreshFamily())
	s.maybeCleanupExpiredRefreshTokens(ctx)

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(TokenResponse{
		AccessToken: access, RefreshToken: refresh, TokenType: "bearer",
	}, ""))
}

// maybePromoteSuperuser：SUPERUSER_EMAILS env（逗号分隔）命中即幂等
// 提升——部署者的 bootstrap 管理通道，不属于代码硬编码。
// 匹配规则与启动同步（SyncSuperusers）共用：全邮箱 trim + 小写。
func (s *Service) maybePromoteSuperuser(ctx context.Context, user *store.User) {
	if user.Email == nil || !isSuperuserEmail(s.Cfg.SuperuserEmails, *user.Email) {
		return
	}
	if !user.IsSuperuser {
		_ = s.DB.WithContext(ctx).Model(user).Update("is_superuser", true).Error
		user.IsSuperuser = true
	}
}

// POST /auth/refresh（JSON {refresh_token}）→ 新 token 对
// 轮换 + 15s 宽限（多标签页并发不误踢）+ 窗口外重放整族吊销。
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
	// 注册表校验（DB 持久化）：行存在、未过期
	oldHash := tokenHash(req.RefreshToken)
	row, ok := s.lookupRefreshToken(ctx, oldHash)
	if !ok {
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
	if claims.AuthVersion != user.AuthVersion {
		httpapi.WriteError(c.Writer, errBad("Refresh token has been revoked", http.StatusUnauthorized))
		return
	}

	now := time.Now().UTC()
	newRefresh, _ := CreateVersionedRefreshToken(s.Cfg.SecretKey, user.ID.String(), user.AuthVersion, s.Cfg.RefreshTokenExpireDays)
	family := row.FamilyID
	if family == nil {
		// 迁移前的历史行还没有 family：首次轮换时补上（与旧行同一次 UPDATE 写回）。
		family = newRefreshFamily()
	}

	if row.RevokedAt == nil {
		won, err := s.rotateRefreshToken(ctx, oldHash, newRefresh, user.ID, family, now)
		if err != nil {
			httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
			return
		}
		if won {
			s.respondWithTokens(c, user, newRefresh)
			return
		}
		// 并发刷新已抢先轮换：必须在事务外重读（MySQL REPEATABLE READ 下
		// 同事务重读会拿到旧快照），再进入宽限/重放判定。
		row, ok = s.lookupRefreshToken(ctx, oldHash)
		if !ok {
			httpapi.WriteError(c.Writer, errBad("Refresh token has been revoked", http.StatusUnauthorized))
			return
		}
		family = row.FamilyID
	}

	if s.graceRefreshAllowed(ctx, &row, now) {
		// 同一 family 再签发一对；不吊销、不改写旧行的 replaced_by。
		s.storeRefresh(ctx, newRefresh, user.ID, s.refreshTTL(), family)
		s.respondWithTokens(c, user, newRefresh)
		return
	}

	// 宽限窗口外重放（或登出后复用）：视为凭证泄露，整族吊销。
	s.revokeRefreshFamily(ctx, &row, now)
	slog.Warn("refresh token replay detected; family revoked",
		"user_id", user.ID.String(), "family_id", familyString(row.FamilyID))
	httpapi.WriteError(c.Writer, errBad("Refresh token has been revoked", http.StatusUnauthorized))
}

// lookupRefreshToken 读取注册表行；行不存在或已过期一律视为不可用。
func (s *Service) lookupRefreshToken(ctx context.Context, hash string) (store.RefreshToken, bool) {
	var row store.RefreshToken
	if err := s.DB.WithContext(ctx).Where("token_hash = ?", hash).First(&row).Error; err != nil {
		return row, false
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		return row, false
	}
	return row, true
}

// rotateRefreshToken 用条件 UPDATE 决出唯一赢家：并发请求只有一个能把旧行
// 从「未吊销」改成「已吊销」，避免双请求同时通过检查后各签一对。
func (s *Service) rotateRefreshToken(ctx context.Context, oldHash, newToken string, uid store.UUID, family *store.UUID, now time.Time) (bool, error) {
	tx := s.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	res := tx.Model(&store.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", oldHash).
		Updates(map[string]any{
			"revoked_at":  now,
			"replaced_by": tokenHash(newToken),
			"family_id":   family,
		})
	if res.Error != nil {
		tx.Rollback()
		return false, res.Error
	}
	if res.RowsAffected != 1 {
		tx.Rollback()
		return false, nil
	}
	newRow := store.RefreshToken{
		TokenHash: tokenHash(newToken),
		UserID:    uid,
		ExpiresAt: now.Add(s.refreshTTL()),
		FamilyID:  family,
	}
	if err := tx.Create(&newRow).Error; err != nil {
		tx.Rollback()
		return false, err
	}
	if err := tx.Commit().Error; err != nil {
		return false, err
	}
	return true, nil
}

// graceRefreshAllowed：宽限期只对「轮换造成的吊销」生效（replaced_by 非空），
// 且 family 仍有活跃成员——登出或整族吊销之后不存在活跃成员，不能复活会话。
func (s *Service) graceRefreshAllowed(ctx context.Context, row *store.RefreshToken, now time.Time) bool {
	if row.RevokedAt == nil || row.ReplacedBy == nil || row.FamilyID == nil {
		return false
	}
	if now.Sub(*row.RevokedAt) > refreshGraceWindow {
		return false
	}
	var active int64
	if err := s.DB.WithContext(ctx).Model(&store.RefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL AND expires_at > ?", row.FamilyID, now).
		Count(&active).Error; err != nil {
		return false
	}
	return active > 0
}

// revokeRefreshFamily 吊销该行所属 family 下全部活跃 token；历史行没有
// family 时只吊销本行（兼容迁移前数据）。
func (s *Service) revokeRefreshFamily(ctx context.Context, row *store.RefreshToken, now time.Time) {
	q := s.DB.WithContext(ctx).Model(&store.RefreshToken{}).Where("revoked_at IS NULL")
	if row.FamilyID != nil {
		q = q.Where("family_id = ?", row.FamilyID)
	} else {
		q = q.Where("token_hash = ?", row.TokenHash)
	}
	_ = q.Update("revoked_at", now).Error
}

func (s *Service) respondWithTokens(c *gin.Context, user store.User, refresh string) {
	access, _ := CreateVersionedAccessToken(s.Cfg.SecretKey, user.ID.String(), user.AuthVersion, s.Cfg.AccessTokenExpireMinutes)
	s.maybeCleanupExpiredRefreshTokens(c.Request.Context())
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(TokenResponse{
		AccessToken: access, RefreshToken: refresh, TokenType: "bearer",
	}, ""))
}

// POST /auth/logout（JSON {refresh_token}）→ 恒 200，吊销整条会话族
// （一次登录 = 一个 family，同浏览器多标签页共享；不整族吊销会留下宽限期
// 内的兄弟 token，登出后仍可续签）。
func (s *Service) logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	ctx := c.Request.Context()
	var row store.RefreshToken
	if err := s.DB.WithContext(ctx).Where("token_hash = ?", tokenHash(req.RefreshToken)).First(&row).Error; err == nil {
		s.revokeRefreshFamily(ctx, &row, time.Now().UTC())
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(nil, ""))
}

// storeRefresh 写注册表行（FirstOrCreate：同 hash 重复写入幂等）
func (s *Service) storeRefresh(ctx context.Context, token string, uid store.UUID, ttl time.Duration, family *store.UUID) {
	row := store.RefreshToken{
		TokenHash: tokenHash(token),
		UserID:    uid,
		ExpiresAt: time.Now().UTC().Add(ttl),
		FamilyID:  family,
	}
	_ = s.DB.WithContext(ctx).Where("token_hash = ?", row.TokenHash).FirstOrCreate(&row).Error
}

// maybeCleanupExpiredRefreshTokens 惰性清理过期行：每进程每小时最多一次。
// 30 天 TTL 下每次 access 过期都会新增一行，不清理会无界增长；只删已过期
// （JWE exp 已保证不可用），保留「已吊销但未过期」行作为重放检测依据。
func (s *Service) maybeCleanupExpiredRefreshTokens(ctx context.Context) {
	s.cleanupMu.Lock()
	if time.Since(s.lastCleanup) < time.Hour {
		s.cleanupMu.Unlock()
		return
	}
	s.lastCleanup = time.Now()
	s.cleanupMu.Unlock()
	_ = s.DB.WithContext(ctx).Where("expires_at < ?", time.Now().UTC()).Delete(&store.RefreshToken{}).Error
}

// newRefreshFamily 生成一个新的会话链 id（登录/验证码登录各一条）。
func newRefreshFamily() *store.UUID {
	id := store.UUID{UUID: uuid.New()}
	return &id
}

func familyString(family *store.UUID) string {
	if family == nil {
		return ""
	}
	return family.String()
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
