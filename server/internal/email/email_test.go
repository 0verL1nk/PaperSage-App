package email_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/email"
)

// Sender 契约：
// - Resend：POST /emails，Bearer 鉴权，{from,to,subject,html} 载荷，
//   非 2xx 返回错误
// - Noop：开发/测试替身，恒成功
// - SMTP：net/smtp 简单实现（由 e2e/手动验证，此处不单测）

func TestResendSenderRequestShape(t *testing.T) {
	var gotAuth, gotPath string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"em-1"}`))
	}))
	defer srv.Close()

	s := email.NewResend("re_test_key", "noreply@mail.papersage.dev", srv.URL)
	err := s.Send(context.Background(), "user@example.com", "验证你的邮箱", "<p>点击链接</p>")
	require.NoError(t, err)

	assert.Equal(t, "/emails", gotPath)
	assert.Equal(t, "Bearer re_test_key", gotAuth)
	assert.Equal(t, "noreply@mail.papersage.dev", body["from"])
	assert.Equal(t, "user@example.com", body["to"])
	assert.Equal(t, "验证你的邮箱", body["subject"])
	assert.Equal(t, "<p>点击链接</p>", body["html"])
}

func TestResendSenderPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid from"}}`))
	}))
	defer srv.Close()

	s := email.NewResend("re_test_key", "noreply@mail.papersage.dev", srv.URL)
	err := s.Send(context.Background(), "user@example.com", "s", "b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "422")
	assert.Contains(t, err.Error(), "invalid from")
}

func TestNoopSenderReportsUnavailable(t *testing.T) {
	s := email.Noop{}
	require.ErrorContains(t, s.Send(context.Background(), "a@b.c", "s", "b"), "not configured")
}

// 邮件头注入回归：CR/LF 或畸形地址必须在建连前被拒绝（CodeQL go/email-injection）。
func TestSMTPSenderRejectsHeaderInjection(t *testing.T) {
	s := &email.SMTP{Host: "127.0.0.1:1", From: "noreply@example.com"}
	for _, to := range []string{
		"",
		"not-an-email",
		"user@example.com\r\nBcc: evil@example.com",
		"user@example.com\nX-Evil: 1",
		"user@example.com ",
	} {
		err := s.Send(context.Background(), to, "s", "b")
		require.Error(t, err, "地址 %q 必须被拒绝", to)
		assert.Contains(t, err.Error(), "invalid address")
	}
}

func TestSMTPSenderAcceptsDisplayAddressSyntax(t *testing.T) {
	// 只验证地址归一化，不真的建连：无效 Host 会在解析之后才失败。
	s := &email.SMTP{Host: "127.0.0.1:1", From: "PaperSage <noreply@example.com>"}
	err := s.Send(context.Background(), "Student <student@example.edu.cn>", "s", "b")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "invalid address")
}
