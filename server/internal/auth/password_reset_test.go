package auth

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestPasswordResetRequestDoesNotEnumerateAndResetRevokesSessions(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	mailer := &captureSender{}
	srv.setEmailSender(t, mailer)

	address := "reset-user@example.com"
	oldPasswordHash, err := HashPassword("old-client-hash")
	require.NoError(t, err)
	user := store.User{
		Username: "reset_user", PasswordHash: oldPasswordHash, Email: &address,
		EmailVerified: true, IsEmailRegistered: true, IsActive: true,
	}
	require.NoError(t, srv.db.Create(&user).Error)

	unknown := postJSON(t, srv.URL+"/api/v1/auth/password-reset/request", map[string]string{"email": "missing@example.com"})
	known := postJSON(t, srv.URL+"/api/v1/auth/password-reset/request", map[string]string{"email": address})
	require.Equal(t, http.StatusOK, unknown.StatusCode)
	require.Equal(t, http.StatusOK, known.StatusCode)
	unknownMessage := bodyJSON(t, unknown)["data"].(map[string]any)["message"]
	knownMessage := bodyJSON(t, known)["data"].(map[string]any)["message"]
	assert.Equal(t, unknownMessage, knownMessage)
	assert.Equal(t, passwordResetOK, knownMessage)

	require.Eventually(t, func() bool { return capturedEmailCount(mailer) == 1 }, time.Second, 10*time.Millisecond)
	mail := capturedEmails(mailer)[0]
	assert.Equal(t, address, mail.to)
	assert.Equal(t, "重置你的 PaperSage 密码", mail.subject)
	assert.Contains(t, mail.html, "/auth/reset-password#token=")
	token := extractVerifyToken(t, mail.html)
	assert.Len(t, token, 43, "32 bytes of entropy use 43 URL-safe Base64 characters")

	var stored store.VerificationToken
	require.NoError(t, srv.db.Where("token_hash = ?", tokenHash(token)).First(&stored).Error)
	assert.Equal(t, passwordResetPurpose, stored.Purpose)
	assert.NotEqual(t, token, stored.TokenHash)
	assert.WithinDuration(t, time.Now().Add(passwordResetTTL), stored.ExpiresAt, 2*time.Second)

	limited := postJSON(t, srv.URL+"/api/v1/auth/password-reset/request", map[string]string{"email": address})
	assert.Equal(t, http.StatusOK, limited.StatusCode)
	assert.Equal(t, passwordResetOK, bodyJSON(t, limited)["data"].(map[string]any)["message"])
	assert.Equal(t, 1, capturedEmailCount(mailer), "repeated requests within the cooldown must not send another email")

	access, err := CreateVersionedAccessToken(testSecret, user.ID.String(), 0, 30)
	require.NoError(t, err)
	refresh, err := CreateVersionedRefreshToken(testSecret, user.ID.String(), 0, 7)
	require.NoError(t, err)
	require.NoError(t, srv.db.Create(&store.RefreshToken{
		TokenHash: tokenHash(refresh), UserID: user.ID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}).Error)

	newPasswordHash := "new-client-hash"
	reset := postJSON(t, srv.URL+"/api/v1/auth/password-reset/confirm", map[string]string{
		"token": token, "password": newPasswordHash,
	})
	require.Equal(t, http.StatusOK, reset.StatusCode, bodyJSON(t, reset)["info"])

	oldSession := authorizedGet(t, srv.URL+"/api/v1/auth/me", access)
	assert.Equal(t, http.StatusUnauthorized, oldSession.StatusCode)
	refreshResp := postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": refresh})
	assert.Equal(t, http.StatusUnauthorized, refreshResp.StatusCode)

	loginOld := postForm(t, srv.URL+"/api/v1/auth/login", url.Values{"account": {address}, "password": {"old-client-hash"}})
	assert.Equal(t, http.StatusUnauthorized, loginOld.StatusCode)
	loginNew := postForm(t, srv.URL+"/api/v1/auth/login", url.Values{"account": {address}, "password": {newPasswordHash}})
	assert.Equal(t, http.StatusOK, loginNew.StatusCode)

	replayed := postJSON(t, srv.URL+"/api/v1/auth/password-reset/confirm", map[string]string{
		"token": token, "password": "another-client-hash",
	})
	assert.Equal(t, http.StatusBadRequest, replayed.StatusCode)
	loginStillNew := postForm(t, srv.URL+"/api/v1/auth/login", url.Values{"account": {address}, "password": {newPasswordHash}})
	assert.Equal(t, http.StatusOK, loginStillNew.StatusCode, "replaying a consumed token must not change the password")
}

func TestPasswordResetRequestSkipsUnverifiedUsersAndRateLimits(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	mailer := &captureSender{}
	srv.setEmailSender(t, mailer)
	address := "unverified@example.com"
	passwordHash, err := HashPassword("client-hash")
	require.NoError(t, err)
	require.NoError(t, srv.db.Create(&store.User{
		Username: "unverified", PasswordHash: passwordHash, Email: &address, IsActive: true,
	}).Error)

	first := postJSON(t, srv.URL+"/api/v1/auth/password-reset/request", map[string]string{"email": address})
	second := postJSON(t, srv.URL+"/api/v1/auth/password-reset/request", map[string]string{"email": address})
	assert.Equal(t, http.StatusOK, first.StatusCode)
	assert.Equal(t, http.StatusOK, second.StatusCode)
	assert.Equal(t, 0, capturedEmailCount(mailer))

	var rows int64
	require.NoError(t, srv.db.Model(&store.VerificationToken{}).Where("purpose = ?", passwordResetPurpose).Count(&rows).Error)
	assert.Zero(t, rows)
}

func TestPasswordResetRejectsExpiredAndEmailVerificationTokens(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	address := "token-purpose@example.com"
	user := store.User{Username: "tokenpurpose", PasswordHash: "hash", Email: &address, EmailVerified: true, IsActive: true}
	require.NoError(t, srv.db.Create(&user).Error)

	for _, testCase := range []struct {
		name      string
		purpose   string
		expiresAt time.Time
	}{
		{name: "expired reset", purpose: passwordResetPurpose, expiresAt: time.Now().Add(-time.Minute)},
		{name: "email verification token", purpose: emailVerifyPurpose, expiresAt: time.Now().Add(time.Hour)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			token := uuid.NewString()
			require.NoError(t, srv.db.Create(&store.VerificationToken{
				TokenHash: tokenHash(token), UserID: user.ID, Purpose: testCase.purpose, ExpiresAt: testCase.expiresAt,
			}).Error)
			resp := postJSON(t, srv.URL+"/api/v1/auth/password-reset/confirm", map[string]string{
				"token": token, "password": "new-client-hash",
			})
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestSendAuthEmailFailureLogsErrorWithMaskedDomain(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := NewService(srv.db, &config.Settings{}, failingSender{err: errors.New("smtp down")})
	svc.sendAuthEmail("reset-user@example.com", "重置你的 PaperSage 密码", `<a href="/auth/reset-password#token=secret-token">`)

	logged := buf.String()
	assert.Contains(t, logged, "level=ERROR")
	assert.Contains(t, logged, "email_domain=example.com")
	assert.NotContains(t, logged, "reset-user@example.com", "full email must not be logged")
	assert.NotContains(t, logged, "reset-user", "local part must not be logged")
	assert.NotContains(t, logged, "secret-token", "reset token must not be logged")
}

func capturedEmails(sender *captureSender) []struct{ to, subject, html string } {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]struct{ to, subject, html string }(nil), sender.sent...)
}

func capturedEmailCount(sender *captureSender) int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.sent)
}

func authorizedGet(t *testing.T, endpoint, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}
