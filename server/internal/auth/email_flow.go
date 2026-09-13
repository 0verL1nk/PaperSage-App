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
const verifyTokenTTL = 24 * time.Hour

// 重发冷却由 limiter.allow("resend:"+email, 1) 实现：窗口即 rateWindow(1min)
// ——resendCooldown 常量保留作语义文档锚点

// ── 邮箱注册 + 验证 ────────────────────────────────────────────

// registerEmailFlow：邮箱注册制（email+password 表单）。
// 防枚举：已注册邮箱与成功注册响应一致（恒 200 + 提示查邮件，
// 已存在时静默不重发——对方若真是主人可走重发/登录路径）。
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
		// 防枚举：响应与成功一致。不自动补发（用户主导）——未收到
		// 邮件时走登录 403 提示条 / 提醒条上的「重新发送」按钮，
		// 或重发端点（均有限速）
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
	s.sendVerification(c.Request.Context(), &user)
	s.silentVerifyOK(c)
}

func (s *Service) silentVerifyOK(c *gin.Context) {
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"message": "注册成功，请查收验证邮件完成激活",
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

// sendVerification 生成一次性 token 并发邮件（失败仅日志——注册流程
// 不因邮件服务故障中断，用户可走重发）。
func (s *Service) sendVerification(ctx context.Context, user *store.User) {
	token, err := CreateVerifyToken(s.Cfg.SecretKey, user.ID.String(), verifyTokenTTL)
	if err != nil {
		return
	}
	// 注册表：hash 存储（与 refresh_tokens 同模式）
	row := store.VerificationToken{
		TokenHash: tokenHash(token),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(verifyTokenTTL),
	}
	_ = s.DB.WithContext(ctx).Create(&row).Error

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
	html := verificationEmailHTML(link, time.Now().Add(verifyTokenTTL))
	if err := mailer.Send(ctx, to, "PaperSage 邮箱验证", html); err != nil {
		// 日志不落 token（OWASP）
		slogWarn("send verification email failed", "email", maskEmail(to), "err", err)
	}
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
	_, err := VerifyToken(token, s.Cfg.SecretKey, "email_verify")
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification link", http.StatusBadRequest))
		return
	}
	var row store.VerificationToken
	err = s.DB.Where("token_hash = ?", tokenHash(token)).First(&row).Error
	if err != nil || time.Now().After(row.ExpiresAt) {
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
	now := time.Now()
	_ = s.DB.Model(&store.VerificationToken{}).
		Where("token_hash = ?", row.TokenHash).Update("used_at", now).Error
	_ = s.DB.Model(&store.User{}).
		Where("id = ?", row.UserID).Update("email_verified", true).Error
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"verified": true}, ""))
}

// resendVerification：POST {email} — 恒 200（防枚举）；真用户 60s 冷却。
func (s *Service) resendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if !s.limiter.allow("resend:"+req.Email, 1) {
		// 冷却期内：同样 200（防枚举 + 防轰炸）
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"message": "如该邮箱已注册且未验证，验证邮件已发送"}, ""))
		return
	}
	email := normalizeEmail(req.Email)
	var user store.User
	err := s.DB.Where("email = ? AND email_verified = false", email).First(&user).Error
	if err == nil {
		s.sendVerification(c.Request.Context(), &user)
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"message": "如该邮箱已注册且未验证，验证邮件已发送"}, ""))
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
