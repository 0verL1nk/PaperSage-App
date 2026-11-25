package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 邮箱验证码登录契约：
// - 6 位码只存 bcrypt 哈希、5 分钟过期、重发作废旧码、60s 冷却、IP/邮箱双维度限流
// - 一次核销（重放 401）、错 5 次作废
// - 新邮箱自动注册（无密码占位、邮箱已验证），老用户直接登录并补验证
// - 自动注册用户可通过 /auth/password 免旧密码设置密码

var loginCodeBodyPattern = regexp.MustCompile(`letter-spacing:6px;font-weight:600;margin:24px 0">(\d{6})</p>`)

func latestLoginCode(t *testing.T, cap *captureSender) string {
	t.Helper()
	require.NotEmpty(t, cap.sent)
	m := loginCodeBodyPattern.FindStringSubmatch(cap.sent[len(cap.sent)-1].html)
	require.Len(t, m, 2, "邮件正文应包含 6 位验证码")
	return m[1]
}

func sendLoginCode(t *testing.T, srv *authServer, email string) *http.Response {
	t.Helper()
	return postJSON(t, srv.URL+"/api/v1/auth/email-code/send", map[string]string{"email": email})
}

func verifyLoginCode(t *testing.T, srv *authServer, email, code string) *http.Response {
	t.Helper()
	return postJSON(t, srv.URL+"/api/v1/auth/email-code/verify", map[string]string{"email": email, "code": code})
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func getWithToken(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req) //nolint:noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func postJSONWithToken(t *testing.T, url, token string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req) //nolint:noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestEmailCodeNewUserLogin(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)
	srv.setupWhitelist(t, "hit.edu.cn")

	resp := sendLoginCode(t, srv, "S001@HIT.EDU.CN")
	require.Equal(t, 200, resp.StatusCode, bodyJSON(t, resp)["info"])
	require.Len(t, cap.sent, 1)
	assert.Equal(t, "s001@hit.edu.cn", cap.sent[0].to, "邮箱应整串小写后发送")

	code := latestLoginCode(t, cap)
	var rows []store.EmailLoginCode
	require.NoError(t, srv.db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.NotEqual(t, code, rows[0].CodeHash, "库里绝不能是明文验证码")
	assert.True(t, VerifyPassword(code, rows[0].CodeHash), "库里应是可核销的 bcrypt 哈希")
	assert.WithinDuration(t, time.Now().UTC().Add(emailCodeTTL), rows[0].ExpiresAt, 5*time.Second)

	// 错码 → 401 且带剩余次数
	resp = verifyLoginCode(t, srv, "s001@hit.edu.cn", wrongCode(code))
	require.Equal(t, 401, resp.StatusCode)
	assert.Contains(t, bodyJSON(t, resp)["info"], "4 attempts left")

	// 正确码 → token + isNewUser
	resp = verifyLoginCode(t, srv, "s001@hit.edu.cn", code)
	verified := bodyJSON(t, resp)
	require.Equal(t, 200, resp.StatusCode, verified["info"])
	data := verified["data"].(map[string]any)
	assert.Equal(t, true, data["isNewUser"])
	access, _ := data["accessToken"].(string)
	require.NotEmpty(t, access)

	var user store.User
	require.NoError(t, srv.db.Where("email = ?", "s001@hit.edu.cn").First(&user).Error)
	assert.True(t, user.EmailVerified)
	assert.True(t, user.IsEmailRegistered)
	assert.True(t, user.IsActive)
	assert.False(t, hasPassword(user.PasswordHash), "自动注册账号不应有可用密码")

	// /auth/me 暴露 hasPassword=false
	resp = getWithToken(t, srv.URL+"/api/v1/auth/me", access)
	require.Equal(t, 200, resp.StatusCode)
	me := bodyJSON(t, resp)["data"].(map[string]any)
	assert.Equal(t, false, me["hasPassword"])
	assert.Equal(t, true, me["emailVerified"])

	// 占位密码不可登录
	resp = postForm(t, srv.URL+"/api/v1/auth/login", url.Values{"account": {"s001@hit.edu.cn"}, "password": {"whatever1"}})
	assert.Equal(t, 401, resp.StatusCode)

	// 同一码重放 → 401（一次性核销）
	resp = verifyLoginCode(t, srv, "s001@hit.edu.cn", code)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestEmailCodeSendCooldown(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	srv.setEmailSender(t, &captureSender{})

	require.Equal(t, 200, sendLoginCode(t, srv, "cool@hit.edu.cn").StatusCode)
	resp := sendLoginCode(t, srv, "cool@hit.edu.cn")
	require.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "60", resp.Header.Get("Retry-After"))
}

func TestEmailCodeSendIPLimit(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	srv.setEmailSender(t, &captureSender{})

	// 直接把该 IP 的额度用完，避免真发 30 封（端点查的是同一个键）。
	for i := 0; i < emailCodePerIPHour; i++ {
		require.True(t, srv.svc.limiter.allowWindow("email-code-ip:127.0.0.1", emailCodePerIPHour, time.Hour))
	}
	resp := sendLoginCode(t, srv, "limited@hit.edu.cn")
	assert.Equal(t, 429, resp.StatusCode)
}

func TestEmailCodeWhitelistRejected(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)
	srv.setupWhitelist(t, "hit.edu.cn")

	resp := sendLoginCode(t, srv, "someone@gmail.com")
	require.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, bodyJSON(t, resp)["info"], "学校邮箱")
	assert.Empty(t, cap.sent)
}

func TestEmailCodeResendInvalidatesOldCode(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	require.Equal(t, 200, sendLoginCode(t, srv, "resend@hit.edu.cn").StatusCode)
	oldCode := latestLoginCode(t, cap)

	srv.svc.limiter = newRateLimiter() // 跳过 60s 冷却，模拟冷却后重发
	require.Equal(t, 200, sendLoginCode(t, srv, "resend@hit.edu.cn").StatusCode)
	newCode := latestLoginCode(t, cap)
	require.NotEqual(t, oldCode, newCode)

	assert.Equal(t, 401, verifyLoginCode(t, srv, "resend@hit.edu.cn", oldCode).StatusCode, "旧码必须作废")
	assert.Equal(t, 200, verifyLoginCode(t, srv, "resend@hit.edu.cn", newCode).StatusCode)

	var count int64
	require.NoError(t, srv.db.Model(&store.EmailLoginCode{}).Where("email = ?", "resend@hit.edu.cn").Count(&count).Error)
	assert.Equal(t, int64(1), count, "已用/过期行应被清理，表不随发送次数增长")
}

func TestEmailCodeExpired(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	require.Equal(t, 200, sendLoginCode(t, srv, "expire@hit.edu.cn").StatusCode)
	code := latestLoginCode(t, cap)
	require.NoError(t, srv.db.Model(&store.EmailLoginCode{}).
		Where("email = ?", "expire@hit.edu.cn").
		Update("expires_at", time.Now().UTC().Add(-time.Second)).Error)

	resp := verifyLoginCode(t, srv, "expire@hit.edu.cn", code)
	require.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Invalid or expired verification code", bodyJSON(t, resp)["info"])
}

func TestEmailCodeLocksAfterFiveWrongAttempts(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	require.Equal(t, 200, sendLoginCode(t, srv, "lock@hit.edu.cn").StatusCode)
	code := latestLoginCode(t, cap)
	wrong := wrongCode(code)

	for i := 1; i < emailCodeMaxAttempts; i++ {
		resp := verifyLoginCode(t, srv, "lock@hit.edu.cn", wrong)
		require.Equal(t, 401, resp.StatusCode)
		assert.Contains(t, bodyJSON(t, resp)["info"], "attempts left")
	}
	resp := verifyLoginCode(t, srv, "lock@hit.edu.cn", wrong)
	require.Equal(t, 401, resp.StatusCode)
	assert.Contains(t, bodyJSON(t, resp)["info"], "Too many incorrect")

	assert.Equal(t, 401, verifyLoginCode(t, srv, "lock@hit.edu.cn", code).StatusCode, "错满 5 次后正确码也不能用")
}

func TestEmailCodeSendFailureInvalidatesCode(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	srv.setEmailSender(t, failingSender{err: errors.New("smtp down")})

	resp := sendLoginCode(t, srv, "down@hit.edu.cn")
	require.Equal(t, 503, resp.StatusCode)

	var row store.EmailLoginCode
	require.NoError(t, srv.db.Where("email = ?", "down@hit.edu.cn").First(&row).Error)
	assert.NotNil(t, row.UsedAt, "发信失败必须作废，避免没收到却能猜")
}

func TestEmailCodeActivatesExistingUnverifiedAccount(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	// 邮箱密码注册（未验证）
	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"email": {"old@hit.edu.cn"}, "password": {"OldPass123"},
	})
	require.Equal(t, 200, resp.StatusCode)

	srv.svc.limiter = newRateLimiter()
	require.Equal(t, 200, sendLoginCode(t, srv, "old@hit.edu.cn").StatusCode)
	code := latestLoginCode(t, cap)

	resp = verifyLoginCode(t, srv, "old@hit.edu.cn", code)
	verified := bodyJSON(t, resp)
	require.Equal(t, 200, resp.StatusCode, verified["info"])
	data := verified["data"].(map[string]any)
	assert.Equal(t, false, data["isNewUser"], "已存在账号不应重复建号")

	var user store.User
	require.NoError(t, srv.db.Where("email = ?", "old@hit.edu.cn").First(&user).Error)
	assert.True(t, user.EmailVerified, "验证码核销应激活未验证账号")
	assert.True(t, hasPassword(user.PasswordHash))

	// 激活后原密码可登录
	resp = postForm(t, srv.URL+"/api/v1/auth/login", url.Values{
		"account": {"old@hit.edu.cn"}, "password": {"OldPass123"},
	})
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEmailCodeCaseInsensitiveMatchesExistingAccount(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	// 存量账号邮箱保留大小写
	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"account": {"Zhang"}, "password": {"OldPass123"}, "email": {"Zhang@hit.edu.cn"},
	})
	require.Equal(t, 200, resp.StatusCode)

	srv.svc.limiter = newRateLimiter()
	require.Equal(t, 200, sendLoginCode(t, srv, "zhang@hit.edu.cn").StatusCode)
	resp = verifyLoginCode(t, srv, "zhang@hit.edu.cn", latestLoginCode(t, cap))
	require.Equal(t, 200, resp.StatusCode)

	var count int64
	require.NoError(t, srv.db.Model(&store.User{}).Where("LOWER(email) = ?", "zhang@hit.edu.cn").Count(&count).Error)
	assert.Equal(t, int64(1), count, "大小写不同不应分裂出第二个账号")
}

func TestEmailCodeDisabledAccount(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	email := "off@hit.edu.cn"
	hash, err := HashPassword("OldPass123")
	require.NoError(t, err)
	off := store.User{
		Username: "off", PasswordHash: hash, Email: &email,
		EmailVerified: true, IsEmailRegistered: true,
	}
	require.NoError(t, srv.db.Create(&off).Error)
	// GORM 对 default:true 的零值会落回默认值，停用状态必须显式更新。
	require.NoError(t, srv.db.Model(&store.User{}).Where("id = ?", off.ID).Update("is_active", false).Error)

	require.Equal(t, 200, sendLoginCode(t, srv, email).StatusCode)
	resp := verifyLoginCode(t, srv, email, latestLoginCode(t, cap))
	require.Equal(t, 403, resp.StatusCode)
	assert.Equal(t, "Account is disabled", bodyJSON(t, resp)["info"])
}

func TestEmailCodeVerifyRejectsMalformedInput(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/auth/email-code/verify", map[string]string{"email": "a@hit.edu.cn", "code": "12345"})
	assert.Equal(t, 422, resp.StatusCode)
	resp = postJSON(t, srv.URL+"/api/v1/auth/email-code/verify", map[string]string{"email": "a@hit.edu.cn", "code": "abcdef"})
	assert.Equal(t, 422, resp.StatusCode)
}

func TestSetPasswordForAutoRegisteredUser(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)

	// 验证码登录自动注册
	require.Equal(t, 200, sendLoginCode(t, srv, "setpw@hit.edu.cn").StatusCode)
	resp := verifyLoginCode(t, srv, "setpw@hit.edu.cn", latestLoginCode(t, cap))
	require.Equal(t, 200, resp.StatusCode)
	access := bodyJSON(t, resp)["data"].(map[string]any)["accessToken"].(string)

	// 免旧密码设置初始密码
	resp = postJSONWithToken(t, srv.URL+"/api/v1/auth/password", access, map[string]string{"newPassword": "NewPass123"})
	saved := bodyJSON(t, resp)
	require.Equal(t, 200, resp.StatusCode, saved["info"])
	assert.Equal(t, true, saved["data"].(map[string]any)["hasPassword"])

	resp = getWithToken(t, srv.URL+"/api/v1/auth/me", access)
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, true, bodyJSON(t, resp)["data"].(map[string]any)["hasPassword"])

	// 新密码可登录
	resp = postForm(t, srv.URL+"/api/v1/auth/login", url.Values{"account": {"setpw@hit.edu.cn"}, "password": {"NewPass123"}})
	require.Equal(t, 200, resp.StatusCode)

	// 已设密码后修改必须校验旧密码
	resp = postJSONWithToken(t, srv.URL+"/api/v1/auth/password", access, map[string]string{
		"currentPassword": "wrong", "newPassword": "Another123",
	})
	require.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "Current password is incorrect", bodyJSON(t, resp)["info"])

	resp = postJSONWithToken(t, srv.URL+"/api/v1/auth/password", access, map[string]string{
		"currentPassword": "NewPass123", "newPassword": "Another123",
	})
	require.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, 401, postForm(t, srv.URL+"/api/v1/auth/login", url.Values{
		"account": {"setpw@hit.edu.cn"}, "password": {"NewPass123"},
	}).StatusCode)
	assert.Equal(t, 200, postForm(t, srv.URL+"/api/v1/auth/login", url.Values{
		"account": {"setpw@hit.edu.cn"}, "password": {"Another123"},
	}).StatusCode)
}

func TestSetPasswordRequiresAuth(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/auth/password", map[string]string{"newPassword": "NewPass123"})
	assert.Equal(t, 401, resp.StatusCode)
}

func TestRateLimiterWindowAndSweep(t *testing.T) {
	r := newRateLimiter()
	assert.True(t, r.allowWindow("k", 2, time.Hour))
	assert.True(t, r.allowWindow("k", 2, time.Hour))
	assert.False(t, r.allowWindow("k", 2, time.Hour), "窗口内超过上限必须拒绝")

	// 惰性清扫：过期条目应在后续调用中被移除，避免内存无界增长。
	r.mu.Lock()
	r.hits["stale"] = &rateEntry{hits: []time.Time{time.Now().Add(-2 * time.Hour)}, window: time.Hour}
	r.lastSweep = time.Now().Add(-2 * time.Minute)
	r.mu.Unlock()
	assert.True(t, r.allowWindow("other", 1, time.Minute))
	r.mu.Lock()
	_, staleKept := r.hits["stale"]
	r.mu.Unlock()
	assert.False(t, staleKept, "过期键必须被清扫")
}
