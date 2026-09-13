package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/api/routes/auth.py + api/dependencies.py

const testSecret = "test-secret-key"

// swappableSender 运行时可替换的邮件替身（auth 测试基座专用）。
type swappableSender struct {
	mu    sync.Mutex
	inner email.Sender
}

func (s *swappableSender) Send(ctx context.Context, to, subject, html string) error {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	return inner.Send(ctx, to, subject, html)
}

func (s *swappableSender) Swap(inner email.Sender) {
	s.mu.Lock()
	s.inner = inner
	s.mu.Unlock()
}

// authServer：带 db/mailer 句柄的测试服务（邮箱注册制测试用）。
type authServer struct {
	*httptest.Server
	db     *gorm.DB
	mailer *swappableSender
}

func (a *authServer) setEmailSender(t *testing.T, s email.Sender) {
	t.Helper()
	a.mailer.Swap(s)
}

// setupWhitelist 直接写 system_settings（测试不 import syssettings，
// 其 Put/匹配语义已有独立覆盖）
func (a *authServer) setupWhitelist(t *testing.T, domains ...string) {
	t.Helper()
	raw, err := json.Marshal(domains)
	require.NoError(t, err)
	require.NoError(t, a.db.Exec(`INSERT INTO system_settings (key, value, updated_at)
		VALUES ('allowed_email_domains', ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw)).Error)
}

func newAuthTestServer(t *testing.T) *authServer {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	mailer := &swappableSender{inner: email.Noop{}}
	svc := NewService(db, &config.Settings{
		SecretKey:                testSecret,
		AccessTokenExpireMinutes: 30,
		RefreshTokenExpireDays:   7,
	}, mailer)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	Mount(v1, svc)
	v1.GET("/me", svc.AuthRequired(), svc.me)

	srv := httptest.NewServer(api)
	return &authServer{Server: srv, db: db, mailer: mailer}
}

func postForm(t *testing.T, url string, form url.Values) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader([]byte(form.Encode()))) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b)) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func bodyJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

func register(t *testing.T, base string) {
	t.Helper()
	resp := postForm(t, base+"/api/v1/auth/register", url.Values{
		"account":  {"alice"},
		"password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	require.Equal(t, 200, resp.StatusCode, "注册应成功")
}

func TestRegisterSuccess(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"account":  {"bob"},
		"password": {"f6e5d4c3b2a1"},
		"email":    {"bob@x.com"},
	})
	assert.Equal(t, 200, resp.StatusCode)

	data := bodyJSON(t, resp)["data"].(map[string]any)
	assert.Regexp(t, `^[0-9a-f-]{36}$`, data["id"], "id 应为带横线 UUID（Pydantic 序列化形态）")
	assert.Equal(t, "bob", data["username"])
	assert.Equal(t, "bob@x.com", data["email"])
	assert.Equal(t, true, data["is_active"])
	assert.Equal(t, false, data["is_superuser"])
}

func TestRegisterDuplicateUsername(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)

	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"account": {"alice"}, "password": {"whatever123456"},
	})
	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "Username already exists", bodyJSON(t, resp)["info"])
}

func TestRegisterMissingField(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{"account": {"x"}})
	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, "请求参数验证失败，请检查输入", bodyJSON(t, resp)["info"])
}

func loginOK(t *testing.T, base string) map[string]any {
	t.Helper()
	resp := postForm(t, base+"/api/v1/auth/login", url.Values{
		"account":  {"alice"},
		"password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	require.Equal(t, 200, resp.StatusCode)
	return bodyJSON(t, resp)["data"].(map[string]any)
}

func TestLoginSuccessShape(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)

	data := loginOK(t, srv.URL)
	assert.NotEmpty(t, data["accessToken"])
	assert.NotEmpty(t, data["refreshToken"])
	assert.Equal(t, "bearer", data["tokenType"], "tokenType 固定值")
}

func TestLoginInvalidCredentials(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)

	for _, tc := range []struct {
		name string
		form url.Values
	}{
		{"未知用户", url.Values{"account": {"nobody"}, "password": {"x"}}},
		{"密码错误", url.Values{"account": {"alice"}, "password": {"wrong"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := postForm(t, srv.URL+"/api/v1/auth/login", tc.form)
			assert.Equal(t, 401, resp.StatusCode)
			assert.Equal(t, "Invalid credentials", bodyJSON(t, resp)["info"])
		})
	}
}

// refresh 流程：换新对；真正的 rotation——旧 refresh 被吊销（坏设计修正：
// Python 只存新不删旧，已修正为吊销旧 token）
func TestRefreshFlow(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	data := loginOK(t, srv.URL)
	oldRefresh := data["refreshToken"].(string)

	resp := postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": oldRefresh})
	require.Equal(t, 200, resp.StatusCode)
	newData := bodyJSON(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, newData["accessToken"])

	// 真正的轮换：旧 refresh 用后即废。此前该断言写反（旧 token 可复用）
	// 且靠 JWT 同秒确定性碰巧通过——JWE 随机 IV 让轮换真正生效后暴露
	resp = postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": oldRefresh})
	assert.Equal(t, 401, resp.StatusCode, "旧 refresh token 应已被吊销")
}

func TestRefreshInvalidToken(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": "garbage"})
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Invalid or expired refresh token", bodyJSON(t, resp)["info"])
}

func TestLogoutRevokesRefresh(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	data := loginOK(t, srv.URL)
	refresh := data["refreshToken"].(string)

	resp := postJSON(t, srv.URL+"/api/v1/auth/logout", map[string]string{"refresh_token": refresh})
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, nil, bodyJSON(t, resp)["data"], "logout data 为 null")

	resp = postJSON(t, srv.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": refresh})
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Refresh token has been revoked", bodyJSON(t, resp)["info"])
}

// get_current_user 中间件：四种 401/500 文案对齐 dependencies.py
func TestAuthRequired(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	access := loginOK(t, srv.URL)["accessToken"].(string)

	get := func(hdr string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/me", nil) //nolint:noctx // test
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	resp := get("")
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Not authenticated", bodyJSON(t, resp)["info"])

	resp = get("Bearer garbage.token.here")
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Invalid token", bodyJSON(t, resp)["info"])

	resp = get("Bearer " + access)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "alice", bodyJSON(t, resp)["data"].(map[string]any)["username"])

	// 有效签名但 sub 非 UUID → 401（坏设计修正：Python 未捕获异常返回 500）
	bad, err := CreateAccessToken(testSecret, "not-a-uuid", 30)
	require.NoError(t, err)
	resp = get("Bearer " + bad)
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Invalid token", bodyJSON(t, resp)["info"])
}
