package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

// EmailGate：注册邮箱域名校验注入点（默认读 system_settings 的
// allowed_email_domains 并匹配；测试可覆写）。白名单存 DB（admin
// 可配），匹配语义：裸域命中自身与任意子域，"*."前缀仅子域，
// 精确段匹配防前后缀伪造。
var EmailGate = emailAllowedFromDB

// allowedDomainsFromDB 读白名单（空列表 = 不限制）。
func allowedDomainsFromDB(db *gorm.DB) []string {
	var raw []byte
	row := db.Table("system_settings").Select("value").
		Where("key = ?", "allowed_email_domains").Row()
	if err := row.Scan(&raw); err != nil || len(raw) == 0 {
		return nil
	}
	var domains []string
	if err := json.Unmarshal(raw, &domains); err != nil {
		return nil
	}
	return domains
}

// emailDomainsHandler：GET /auth/email-domains（公开）——注册页展示
// 支持哪些邮箱域，避免用户注册了才被拒。
func (s *Service) emailDomainsHandler(c *gin.Context) {
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"domains": allowedDomainsFromDB(s.DB),
	}, ""))
}

func emailAllowedFromDB(db *gorm.DB, email string) (bool, string) {
	domains := allowedDomainsFromDB(db)
	if len(domains) == 0 {
		return true, ""
	}
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false, "邮箱格式无效"
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "*.") {
			if strings.HasSuffix(domain, "."+d[2:]) {
				return true, ""
			}
			continue
		}
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true, ""
		}
	}
	return false, "邮箱域名不在允许列表内（需学校邮箱）"
}

// ── 登录/验证端点限速（OWASP/NIST 800-63B：5-10 次/分钟）────────
//
// 内存滑动窗口计数（单实例部署足够；多实例时换 Redis——与 cache 域
// 的取舍一致）。键 = 类型 + 标识（IP 或账号）。窗口与阈值是常量：
// 这是限速器自身的工程参数（如同 bcrypt cost），不是业务配置。

const (
	rateWindow   = time.Minute
	rateMaxLogin = 8 // 略宽于 OWASP 下限，兼顾多人共享校园出口 IP
)

type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}}
}

// allow 记录一次尝试；返回 false 表示超出窗口限额。
func (r *rateLimiter) allow(key string, max int) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.hits[key]
	keep := list[:0]
	for _, t := range list {
		if now.Sub(t) < rateWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) >= max {
		r.hits[key] = keep
		return false
	}
	r.hits[key] = append(keep, now)
	return true
}

// 邮箱验证 token TTL（24h：注册验证通行值；密码重置类高危动作应更短）
const (
	emailVerifyPurpose = "email_verify"
	verifyTokenTTL     = 24 * time.Hour
)

// 重发冷却由 limiter.allow("resend:"+email, 1) 实现：窗口即 rateWindow(1min)
// ——resendCooldown 常量保留作语义文档锚点

// ── 邮箱注册 + 验证 ────────────────────────────────────────────

// registerEmailFlow：邮箱注册制（email+password 表单）。
// 已存在的未验证账号允许限速重发；已验证账号维持统一响应，避免枚举。
func (s *Service) registerEmailFlow(c *gin.Context, email, password string) {
	// 域名白名单（admin 可配；未配置 = 不限制）
	if ok, reason := EmailGate(s.DB, email); !ok {
		httpapi.WriteError(c.Writer, errBad(reason, http.StatusBadRequest))
		return
	}
	// 规范化：域名小写（OWASP：域不区分大小写；本地部分保留原样）
	email = normalizeEmail(email)

	var existing store.User
	err := s.DB.Where("email = ?", email).First(&existing).Error
	if err == nil {
		if !existing.EmailVerified {
			if !s.limiter.allow("resend:"+email, 1) {
				httpapi.WriteError(c.Writer, errBad("Verification email is cooling down; please try again shortly.", http.StatusTooManyRequests))
				return
			}
			if err := s.sendVerification(c.Request.Context(), &existing); err != nil {
				s.writeVerificationSendError(c, email, err)
				return
			}
		}
		s.silentVerifyOK(c)
		return
	}
	hash, err := HashPassword(password)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	username, err := s.uniqueUsernameFromEmail(email)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	user := store.User{Username: username, PasswordHash: hash, Email: &email, IsEmailRegistered: true}
	if err := s.DB.Create(&user).Error; err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	if err := s.sendVerification(c.Request.Context(), &user); err != nil {
		s.writeVerificationSendError(c, email, err)
		return
	}
	s.silentVerifyOK(c)
}

func (s *Service) writeVerificationSendError(c *gin.Context, to string, err error) {
	// Never log tokens or provider credentials; the masked address helps correlate failures.
	slogWarn("send verification email failed", "email", maskEmail(to), "err", err)
	httpapi.WriteError(c.Writer, errBad("Verification email could not be sent. Please try again shortly.", http.StatusServiceUnavailable))
}

func (s *Service) silentVerifyOK(c *gin.Context) {
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"message": "如果该邮箱需要验证，请查收邮件或使用重发功能",
	}, ""))
}

// normalizeEmail：域名部分小写，本地部分保留。
func normalizeEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}
	return email[:at] + "@" + strings.ToLower(email[at+1:])
}

// uniqueUsernameFromEmail：email 本地部分生成 username，冲突加短随机后缀。
func (s *Service) uniqueUsernameFromEmail(email string) (string, error) {
	local := email[:strings.Index(email, "@")]
	local = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			return r
		default:
			return '_'
		}
	}, local)
	if len(local) > 30 {
		local = local[:30]
	}
	base := local
	for i := 0; i < 5; i++ {
		candidate := base
		if i > 0 {
			candidate = base + "_" + randSuffix(4)
		}
		var n int64
		if err := s.DB.Model(&store.User{}).Where("username = ?", candidate).Count(&n).Error; err != nil {
			return "", err
		}
		if n == 0 {
			return candidate, nil
		}
	}
	return base + "_" + randSuffix(8), nil
}

// sendVerification 仅在验证 token 已持久化且邮件服务接受发送后返回成功。
func (s *Service) sendVerification(ctx context.Context, user *store.User) error {
	token, err := CreateVerifyToken(s.Cfg.SecretKey, user.ID.String(), verifyTokenTTL)
	if err != nil {
		return err
	}
	// 注册表：hash 存储（与 refresh_tokens 同模式）
	// 过期时刻以 UTC 写入；邮件展示复用同一时刻（HTML 内转北京时间）
	expiresAt := time.Now().UTC().Add(verifyTokenTTL)
	row := store.VerificationToken{
		TokenHash: tokenHash(token),
		UserID:    user.ID,
		Purpose:   emailVerifyPurpose,
		ExpiresAt: expiresAt,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}

	to := ""
	if user.Email != nil {
		to = *user.Email
	}
	// 热生效：管理后台的 app_url / email 组覆盖启动配置
	appURL := syssettings.GetAppURL(s.DB)
	if appURL == "" {
		appURL = s.Cfg.AppURL
	}
	mailer := s.resolveMailer()
	link := appURL + "/auth/verify?token=" + token
	html := verificationEmailHTML(link, expiresAt)
	return mailer.Send(ctx, to, "PaperSage 邮箱验证", html)
}

// 邮件客户端禁 JS，无法实时倒计时；通行做法是标注失效时刻。
func verificationEmailHTML(link string, expiresAt time.Time) string {
	deadline := expiresAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04")
	return `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto;padding:32px">
<h2 style="color:#1f2328">验证你的 PaperSage 邮箱</h2>
<p>点击下方按钮完成注册验证：</p>
<p style="margin:24px 0">
<a href="` + link + `" style="background:#1f2328;color:#fff;padding:10px 24px;border-radius:8px;text-decoration:none;display:inline-block">验证邮箱</a>
</p>
<p style="color:#6b7280;font-size:13px">链接有效期至北京时间 ` + deadline + `（24 小时）。<br>如果你没有注册 PaperSage，请忽略本邮件。</p>
</div>`
}

// maskEmail：日志脱敏（OWASP：邮箱是敏感标识符）
func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 1 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}

// verifyEmail：GET /auth/verify-email?token= — 一次性验证。
func (s *Service) verifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	claims, err := VerifyToken(token, s.Cfg.SecretKey, "email_verify")
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification link", http.StatusBadRequest))
		return
	}
	var row store.VerificationToken
	err = s.DB.Where("token_hash = ? AND purpose = ?", tokenHash(token), emailVerifyPurpose).First(&row).Error
	if err != nil || row.UserID.String() != claims.Subject || time.Now().UTC().After(row.ExpiresAt) {
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification link", http.StatusBadRequest))
		return
	}
	// 幂等：已用 token 若其用户确已验证 → 200（防邮件预取/双击/
	// React StrictMode 双发把成功体验变成 400；token 绑定 user，
	// 不构成对他人账号的重放）
	if row.UsedAt != nil {
		var u store.User
		if s.DB.Where("id = ?", row.UserID).First(&u).Error == nil && u.EmailVerified {
			_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"verified": true, "already": true}, ""))
			return
		}
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification link", http.StatusBadRequest))
		return
	}
	now := time.Now().UTC()
	err = s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var locked store.VerificationToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ? AND purpose = ?", row.TokenHash, emailVerifyPurpose).First(&locked).Error; err != nil {
			return err
		}
		if locked.UsedAt != nil || now.After(locked.ExpiresAt) {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&store.VerificationToken{}).Where("token_hash = ? AND used_at IS NULL", row.TokenHash).Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&store.User{}).Where("id = ?", row.UserID).Update("email_verified", true).Error
	})
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification link", http.StatusBadRequest))
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"verified": true}, ""))
}

// resendVerification：POST {email} — 未知邮箱保持防枚举响应；真用户 60s 冷却。
func (s *Service) resendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if !s.limiter.allow("resend:"+req.Email, 1) {
		httpapi.WriteError(c.Writer, errBad("Verification email is cooling down; please try again shortly.", http.StatusTooManyRequests))
		return
	}
	email := normalizeEmail(req.Email)
	var user store.User
	err := s.DB.Where("email = ? AND email_verified = false", email).First(&user).Error
	if err == nil {
		if err := s.sendVerification(c.Request.Context(), &user); err != nil {
			s.writeVerificationSendError(c, email, err)
			return
		}
	} else if err != gorm.ErrRecordNotFound {
		httpapi.WriteError(c.Writer, errBad("Unable to check verification status. Please try again shortly.", http.StatusServiceUnavailable))
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"message": "如果该邮箱已注册且未验证，我们会向你发送验证邮件"}, ""))
}

func slogWarn(msg string, kv ...any) { slog.Warn(msg, kv...) }

// randSuffix：CSPRNG 十六进制后缀（用户名防撞用）
func randSuffix(n int) string {
	b := make([]byte, n/2+n%2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// resolveMailer 每次发信时解析：管理后台 email 组 → 启动时注入的 mailer。
func (s *Service) resolveMailer() email.Sender {
	e := syssettings.GetEmail(s.DB)
	switch e.Provider {
	case "resend":
		if e.ResendAPIKey != "" && e.ResendDomain != "" {
			from := "PaperSage <noreply@" + e.ResendDomain + ">"
			return email.NewResend(e.ResendAPIKey, from, "https://api.resend.com")
		}
	case "noop":
		return email.Noop{}
	}
	return s.Mailer
}
