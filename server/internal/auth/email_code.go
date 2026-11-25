package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 邮箱验证码登录：6 位数字、bcrypt 存哈希、5 分钟有效、一次核销；
// 首次核销自动注册（无需密码）。与密码登录共用 JWE 双 token 签发。
const (
	emailCodeTTL          = 5 * time.Minute
	emailCodeMaxAttempts  = 5
	emailCodeCooldown     = time.Minute
	emailCodePerEmailHour = 5
	emailCodePerIPHour    = 30
	emailCodeVerifyPerIP  = 30
	emailCodeVerifyWindow = 10 * time.Minute
	passwordSetPerUser    = 10
	passwordSetWindow     = 10 * time.Minute

	// unusablePasswordPrefix：自动注册账号的密码占位前缀（Django unusable
	// password 约定）。bcrypt 校验必然失败，且无需为一个用不到的密码付哈希成本。
	unusablePasswordPrefix = "!"
)

var (
	emailCodePattern = regexp.MustCompile(`^\d{6}$`)

	errEmailCodeInvalid   = errors.New("email code invalid")
	errEmailCodeExpired   = errors.New("email code expired")
	errEmailCodeExhausted = errors.New("email code attempts exhausted")
	errEmailAccountOff    = errors.New("email account disabled")
	errEmailAccountGone   = errors.New("email account unavailable")
)

// hasPassword：密码哈希存在且不是自动注册占位。
func hasPassword(hash string) bool {
	return hash != "" && !strings.HasPrefix(hash, unusablePasswordPrefix)
}

// emailCodeMismatch 携带剩余可试次数，供前端展示「还可尝试 N 次」。
type emailCodeMismatch struct{ remaining int }

func (e *emailCodeMismatch) Error() string { return "incorrect verification code" }

// POST /auth/email-code/send — 发送 6 位登录验证码。
// 同步发信：用户必须马上知道是否发出；未知邮箱同样发码（核销时自动注册），
// 不存在账号枚举问题。
func (s *Service) sendEmailCode(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if ok, reason := EmailGate(s.DB, email); !ok {
		httpapi.WriteError(c.Writer, errBad(reason, http.StatusBadRequest))
		return
	}
	// IP 维度先于邮箱维度：单个 IP 每小时最多建 30 个邮箱键，内存有界。
	if !s.limiter.allowWindow("email-code-ip:"+c.ClientIP(), emailCodePerIPHour, time.Hour) ||
		!s.limiter.allowWindow("email-code-email:"+email, emailCodePerEmailHour, time.Hour) ||
		!s.limiter.allowWindow("email-code-cooldown:"+email, 1, emailCodeCooldown) {
		s.writeEmailCodeThrottled(c, emailCodeCooldown)
		return
	}

	code, err := newEmailLoginCode()
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	hash, err := HashPassword(code)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	now := time.Now().UTC()
	row := store.EmailLoginCode{Email: email, CodeHash: hash, ExpiresAt: now.Add(emailCodeTTL), CreatedAt: now}
	if err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// 重发即作废旧码；顺手清掉该邮箱的过期/已用行，表不随发送次数增长。
		if err := tx.Model(&store.EmailLoginCode{}).
			Where("email = ? AND used_at IS NULL", email).
			Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Where("email = ? AND (used_at IS NOT NULL OR expires_at < ?)", email, now).
			Delete(&store.EmailLoginCode{}).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	}); err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}

	if err := s.sendLoginCodeEmail(c.Request.Context(), email, code, row.ExpiresAt); err != nil {
		// 发信失败立刻作废，避免"没收到却能猜"的窗口。
		_ = s.DB.WithContext(c.Request.Context()).Model(&store.EmailLoginCode{}).
			Where("id = ?", row.ID).Update("used_at", time.Now().UTC()).Error
		slogWarn("send login code email failed", "email", maskEmail(email), "err", err)
		httpapi.WriteError(c.Writer, errBad("Login code email could not be sent. Please try again shortly.", http.StatusServiceUnavailable))
		return
	}

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"message":         "验证码已发送，请查收邮件。",
		"cooldownSeconds": int(emailCodeCooldown.Seconds()),
	}, ""))
}

func (s *Service) writeEmailCodeThrottled(c *gin.Context, retryAfter time.Duration) {
	c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
	httpapi.WriteError(c.Writer, errBad("Too many verification code requests. Please try again later.", http.StatusTooManyRequests))
}

// POST /auth/email-code/verify — 核销验证码并登录；未知邮箱自动注册。
func (s *Service) verifyEmailCode(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	if email == "" || !emailCodePattern.MatchString(code) {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if !s.limiter.allowWindow("email-code-verify-ip:"+c.ClientIP(), emailCodeVerifyPerIP, emailCodeVerifyWindow) {
		s.writeEmailCodeThrottled(c, emailCodeVerifyWindow)
		return
	}

	var user store.User
	isNewUser := false
	// authErr 承载"必须提交计数后再返回"的失败（错码/次数用尽）：
	// 若直接把错误返回给 Transaction，attempts 的写入会被回滚，锁定永远不生效。
	var authErr error
	err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var row store.EmailLoginCode
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("email = ? AND used_at IS NULL", email).
			Order("created_at DESC").First(&row).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				compareDummyCode(code) // 无待核销码也跑一次 bcrypt，抹平时序差
				return errEmailCodeInvalid
			}
			return err
		}
		now := time.Now().UTC()
		if now.After(row.ExpiresAt) {
			return errEmailCodeExpired
		}
		if !VerifyPassword(code, row.CodeHash) {
			attempts := row.Attempts + 1
			updates := map[string]any{"attempts": attempts}
			if attempts >= emailCodeMaxAttempts {
				updates["used_at"] = now
			}
			if err := tx.Model(&store.EmailLoginCode{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
			if attempts >= emailCodeMaxAttempts {
				authErr = errEmailCodeExhausted
			} else {
				authErr = &emailCodeMismatch{remaining: emailCodeMaxAttempts - attempts}
			}
			return nil
		}
		// 一次性核销：条件更新保证并发重放只有一个请求成功。
		consumed := tx.Model(&store.EmailLoginCode{}).
			Where("id = ? AND used_at IS NULL", row.ID).Update("used_at", now)
		if consumed.Error != nil {
			return consumed.Error
		}
		if consumed.RowsAffected != 1 {
			return errEmailCodeInvalid
		}

		// 大小写不敏感匹配：验证码链路整串小写，避免 Zhang@/zhang@ 分裂账号。
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ?", email).First(&user).Error
		switch {
		case err == nil:
			if !user.IsActive {
				return errEmailAccountOff
			}
			if !user.EmailVerified {
				if err := tx.Model(&store.User{}).Where("id = ?", user.ID).Update("email_verified", true).Error; err != nil {
					return err
				}
				user.EmailVerified = true
			}
			return nil
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 白名单可能在发码后收窄，建号前再校验一次。
			if ok, reason := EmailGate(tx, email); !ok {
				return errBad(reason, http.StatusBadRequest)
			}
			username, err := s.uniqueUsernameFromEmail(tx, email)
			if err != nil {
				return err
			}
			created := store.User{
				Username:          username,
				PasswordHash:      unusablePasswordPrefix + randSuffix(32),
				Email:             &email,
				EmailVerified:     true,
				IsEmailRegistered: true,
				IsActive:          true,
			}
			if err := tx.Create(&created).Error; err != nil {
				// 并发双建：另一事务已建号则复用；否则多半是软删账号占着唯一键。
				if e := tx.Where("LOWER(email) = ?", email).First(&user).Error; e == nil {
					if !user.IsActive {
						return errEmailAccountOff
					}
					return nil
				}
				return errEmailAccountGone
			}
			user = created
			isNewUser = true
			return nil
		default:
			return err
		}
	})
	if err != nil {
		s.writeEmailCodeVerifyError(c, err)
		return
	}
	if authErr != nil {
		s.writeEmailCodeVerifyError(c, authErr)
		return
	}

	ctx := c.Request.Context()
	s.maybePromoteSuperuser(ctx, &user)
	sub := user.ID.String()
	access, err := CreateVersionedAccessToken(s.Cfg.SecretKey, sub, user.AuthVersion, s.Cfg.AccessTokenExpireMinutes)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	refresh, err := CreateVersionedRefreshToken(s.Cfg.SecretKey, sub, user.AuthVersion, s.Cfg.RefreshTokenExpireDays)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	s.storeRefresh(ctx, refresh, user.ID, s.refreshTTL())

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(struct {
		TokenResponse
		IsNewUser bool `json:"isNewUser"`
	}{TokenResponse{AccessToken: access, RefreshToken: refresh, TokenType: "bearer"}, isNewUser}, ""))
}

func (s *Service) writeEmailCodeVerifyError(c *gin.Context, err error) {
	var mismatch *emailCodeMismatch
	var apiErr *httpapi.APIError
	switch {
	case errors.As(err, &mismatch):
		httpapi.WriteError(c.Writer, errBad(
			fmt.Sprintf("Incorrect verification code. %d attempts left.", mismatch.remaining),
			http.StatusUnauthorized))
	case errors.Is(err, errEmailCodeExhausted):
		httpapi.WriteError(c.Writer, errBad("Too many incorrect verification codes. Request a new one.", http.StatusUnauthorized))
	case errors.Is(err, errEmailCodeExpired), errors.Is(err, errEmailCodeInvalid):
		httpapi.WriteError(c.Writer, errBad("Invalid or expired verification code", http.StatusUnauthorized))
	case errors.Is(err, errEmailAccountOff):
		httpapi.WriteError(c.Writer, errBad("Account is disabled", http.StatusForbidden))
	case errors.Is(err, errEmailAccountGone):
		httpapi.WriteError(c.Writer, errBad("This email is linked to an unavailable account. Please contact support.", http.StatusConflict))
	case errors.As(err, &apiErr):
		httpapi.WriteError(c.Writer, apiErr)
	default:
		slogWarn("email code verification failed", "err", err)
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
	}
}

// POST /auth/password — 已登录用户设置/修改密码。
// hasPassword=false（验证码自动注册）时无需旧密码；已设密码时必须校验旧密码。
func (s *Service) setPassword(c *gin.Context) {
	u := CurrentUser(c)
	if u == nil {
		httpapi.WriteError(c.Writer, errBad("Not authenticated", http.StatusUnauthorized))
		return
	}
	if !s.limiter.allowWindow("password-set:"+u.ID.String(), passwordSetPerUser, passwordSetWindow) {
		s.writeEmailCodeThrottled(c, passwordSetWindow)
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.NewPassword == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	// 前端传 SHA-256（与登录一致），强度规则由前端 zod 校验；服务端只见哈希。
	if hasPassword(u.PasswordHash) && (req.CurrentPassword == "" || !VerifyPassword(req.CurrentPassword, u.PasswordHash)) {
		httpapi.WriteError(c.Writer, errBad("Current password is incorrect", http.StatusBadRequest))
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	if err := s.DB.WithContext(c.Request.Context()).Model(&store.User{}).
		Where("id = ?", u.ID).Update("password_hash", hash).Error; err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"message":     "密码已保存，之后可用密码登录。",
		"hasPassword": true,
	}, ""))
}

// newEmailLoginCode：crypto/rand 均匀取 6 位数字（禁止 math/rand）。
func newEmailLoginCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

var (
	dummyCodeHashOnce sync.Once
	dummyCodeHash     string
)

// compareDummyCode 让"没有待核销码"与"有码但输错"耗时接近。
func compareDummyCode(code string) {
	dummyCodeHashOnce.Do(func() { dummyCodeHash, _ = HashPassword("000000") })
	_ = VerifyPassword(code, dummyCodeHash)
}

func (s *Service) sendLoginCodeEmail(ctx context.Context, to, code string, expiresAt time.Time) error {
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	deadline := expiresAt.In(time.FixedZone("CST", 8*3600)).Format("15:04")
	body := `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto;padding:32px">
<h2 style="color:#1f2328">你的 PaperSage 登录验证码</h2>
<p style="font-size:28px;letter-spacing:6px;font-weight:600;margin:24px 0">` + code + `</p>
<p>验证码 5 分钟内有效（至北京时间 ` + deadline + `），仅可使用一次。</p>
<p style="color:#6b7280;font-size:13px">如果这不是你本人操作，请忽略本邮件，你的账号不会发生变化。</p>
</div>`
	return s.resolveMailer().Send(sendCtx, to, "PaperSage 登录验证码", body)
}
