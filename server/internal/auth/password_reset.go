package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

const (
	passwordResetPurpose = "password_reset"
	passwordResetTTL     = 15 * time.Minute
	passwordResetOK      = "如果该邮箱已验证并关联 PaperSage 账号，我们会发送密码重置链接。"
)

var errInvalidPasswordResetToken = errors.New("invalid password reset token")

// requestPasswordReset always returns the same response for eligible,
// unknown, and unverified addresses. The email is sent asynchronously so the
// response does not wait on the mail provider and reveal account state.
func (s *Service) requestPasswordReset(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	emailAddress := normalizeEmail(strings.TrimSpace(req.Email))
	allowed := s.limiter.allow("password-reset-ip:"+c.ClientIP(), 10)
	allowed = s.limiter.allow("password-reset-email:"+emailAddress, 1) && allowed
	if !allowed {
		s.writePasswordResetRequestOK(c)
		return
	}

	var user store.User
	err := s.DB.WithContext(c.Request.Context()).Where(
		"email = ? AND email_verified = ? AND is_active = ?", emailAddress, true, true,
	).First(&user).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			slog.Warn("password reset lookup failed", "err", err)
		}
		s.writePasswordResetRequestOK(c)
		return
	}

	rawToken, err := newPasswordResetToken()
	if err != nil {
		slog.Error("password reset token generation failed", "email", maskEmail(emailAddress), "err", err)
		s.writePasswordResetRequestOK(c)
		return
	}
	expiresAt := time.Now().UTC().Add(passwordResetTTL)
	row := store.VerificationToken{TokenHash: tokenHash(rawToken), UserID: user.ID, Purpose: passwordResetPurpose, ExpiresAt: expiresAt}
	if err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&store.VerificationToken{}).
			Where("user_id = ? AND purpose = ? AND used_at IS NULL", user.ID, passwordResetPurpose).
			Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	}); err != nil {
		slog.Error("password reset token persistence failed", "email", maskEmail(emailAddress), "err", err)
		s.writePasswordResetRequestOK(c)
		return
	}

	s.writePasswordResetRequestOK(c)
	go s.sendPasswordResetMessage(user, rawToken, expiresAt)
}

func (s *Service) writePasswordResetRequestOK(c *gin.Context) {
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"message": passwordResetOK}, ""))
}

func (s *Service) confirmPasswordReset(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || req.Password == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}

	now := time.Now().UTC()
	var changedUser store.User
	err = s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var reset store.VerificationToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"token_hash = ? AND purpose = ? AND used_at IS NULL", tokenHash(req.Token), passwordResetPurpose,
		).First(&reset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidPasswordResetToken
			}
			return err
		}
		if now.After(reset.ExpiresAt) {
			return errInvalidPasswordResetToken
		}
		var user store.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reset.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidPasswordResetToken
			}
			return err
		}
		if !user.IsActive || !user.EmailVerified || user.Email == nil {
			return errInvalidPasswordResetToken
		}
		updated := tx.Model(&store.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"password_hash": passwordHash,
			"auth_version":  gorm.Expr("auth_version + 1"),
		})
		if updated.Error != nil || updated.RowsAffected != 1 {
			if updated.Error != nil {
				return updated.Error
			}
			return errInvalidPasswordResetToken
		}
		if err := tx.Model(&store.VerificationToken{}).
			Where("user_id = ? AND purpose = ? AND used_at IS NULL", user.ID, passwordResetPurpose).
			Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		changedUser = user
		return nil
	})
	if err != nil {
		if errors.Is(err, errInvalidPasswordResetToken) {
			httpapi.WriteError(c.Writer, errBad("Invalid or expired password reset link", http.StatusBadRequest))
			return
		}
		slog.Error("password reset transaction failed", "err", err)
		httpapi.WriteError(c.Writer, errBad("Internal server error", http.StatusInternalServerError))
		return
	}
	go s.sendPasswordChangedMessage(changedUser)
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"message": "密码已重置，请使用新密码登录。"}, ""))
}

func newPasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (s *Service) sendPasswordResetMessage(user store.User, token string, expiresAt time.Time) {
	if user.Email == nil {
		return
	}
	appURL := syssettings.GetAppURL(s.DB)
	if appURL == "" {
		appURL = s.Cfg.AppURL
	}
	link := strings.TrimRight(appURL, "/") + "/auth/reset-password#token=" + token
	deadline := expiresAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04")
	body := `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto;padding:32px">
<h2 style="color:#1f2328">重置你的 PaperSage 密码</h2>
<p>点击下方按钮设置新密码：</p>
<p style="margin:24px 0"><a href="` + html.EscapeString(link) + `" style="background:#1f2328;color:#fff;padding:10px 24px;border-radius:8px;text-decoration:none;display:inline-block">设置新密码</a></p>
<p style="color:#6b7280;font-size:13px">链接有效期至北京时间 ` + deadline + `（15 分钟）。如果你没有申请重置密码，请忽略本邮件；当前密码不会改变。</p>
</div>`
	s.sendAuthEmail(*user.Email, "重置你的 PaperSage 密码", body)
}

func (s *Service) sendPasswordChangedMessage(user store.User) {
	if user.Email == nil {
		return
	}
	body := `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto;padding:32px">
<h2 style="color:#1f2328">PaperSage 密码已更改</h2>
<p>你的账号密码刚刚完成重置。如果这不是你本人操作，请尽快联系 PaperSage 支持。</p>
</div>`
	s.sendAuthEmail(*user.Email, "PaperSage 密码已更改", body)
}

func (s *Service) sendAuthEmail(to, subject, body string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.resolveMailer().Send(ctx, to, subject, body); err != nil {
		// 只记收件域：完整邮箱与正文里的 token 都不能进日志。
		slog.Error("auth email delivery failed", "email_domain", emailDomain(to), "err", err)
	}
}

// emailDomain 返回收件域用于日志；无法解析时返回 "***"。
func emailDomain(address string) string {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "***"
	}
	return address[at+1:]
}
