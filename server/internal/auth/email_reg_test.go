package auth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/email"
)

// 邮箱注册制契约（用户需求：学校邮箱注册 + 域名白名单 + 邮箱验证）：
// - 注册带 email → username 自动生成、email_verified=false、发验证邮件
// - 白名单（system_settings）外的域名 → 400
// - 未验证登录 → 403 Email not verified
// - 验证链接 token 一次性；验证后可登录
// - 重发验证邮件

// captureSender 测试替身：捕获发出的邮件。
type captureSender struct {
	mu   sync.Mutex
	sent []struct{ to, subject, html string }
}

func (c *captureSender) Send(_ context.Context, to, subject, html string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, struct{ to, subject, html string }{to, subject, html})
	return nil
}

func TestEmailRegistrationFlow(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()

	cap := &captureSender{}
	srv.setEmailSender(t, cap)
	srv.setupWhitelist(t, "hit.edu.cn")

	// 非学校邮箱 → 400
	resp := postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"email": {"a@gmail.com"}, "password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, bodyJSON(t, resp)["info"], "学校邮箱")

	// 学校邮箱注册成功 → 发验证邮件、未验证状态
	resp = postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"email": {"s001@hit.edu.cn"}, "password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	require.Equal(t, 200, resp.StatusCode, bodyJSON(t, resp)["info"])

	require.Len(t, cap.sent, 1)
	assert.Equal(t, "s001@hit.edu.cn", cap.sent[0].to)
	assert.Contains(t, cap.sent[0].html, "/auth/verify?token=")

	// 未验证登录 → 403
	resp = postForm(t, srv.URL+"/api/v1/auth/login", url.Values{
		"account": {"s001@hit.edu.cn"}, "password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	assert.Equal(t, 403, resp.StatusCode)
	assert.Contains(t, bodyJSON(t, resp)["info"], "verify your email")

	// 从邮件里抽 token 完成验证
	token := extractVerifyToken(t, cap.sent[0].html)
	resp = httpGet(t, srv.URL+"/api/v1/auth/verify-email?token="+url.QueryEscape(token))
	require.Equal(t, 200, resp.StatusCode)

	// 验证后可登录（email 或自动生成的 username 均可）
	resp = postForm(t, srv.URL+"/api/v1/auth/login", url.Values{
		"account": {"s001@hit.edu.cn"}, "password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	assert.Equal(t, 200, resp.StatusCode)

	// token 一次性 + 幂等：重放时用户已验证 → 200 {already:true}
	//（防 StrictMode 双发/邮件预取把成功变 400）
	resp = httpGet(t, srv.URL+"/api/v1/auth/verify-email?token="+url.QueryEscape(token))
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, true, bodyJSON(t, resp)["data"].(map[string]any)["already"])
}

func TestResendVerification(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	cap := &captureSender{}
	srv.setEmailSender(t, cap)
	srv.setupWhitelist(t, "hit.edu.cn")

	postForm(t, srv.URL+"/api/v1/auth/register", url.Values{
		"email": {"s002@hit.edu.cn"}, "password": {"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"},
	})
	require.Len(t, cap.sent, 1)

	resp := postJSON(t, srv.URL+"/api/v1/auth/resend-verification", map[string]string{"email": "s002@hit.edu.cn"})
	require.Equal(t, 200, resp.StatusCode)
	assert.Len(t, cap.sent, 2, "应重发验证邮件")
}

func extractVerifyToken(t *testing.T, html string) string {
	t.Helper()
	i := strings.Index(html, "token=")
	require.GreaterOrEqual(t, i, 0, "邮件应包含验证链接")
	rest := html[i+len("token="):]
	end := strings.IndexAny(rest, "\"&<")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}

var _ = email.Noop{}

func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}
