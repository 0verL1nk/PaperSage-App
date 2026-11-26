package auth

import (
	"bytes"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer：handler 在服务端 goroutine 写日志，测试在客户端读，需自带锁。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// DEV_LOG_EMAIL_CODE=false 时日志绝不出现验证码；=true 时打印收件邮箱与 6 位码。
func TestEmailCodeSendLogsCodeOnlyWhenDevFlagEnabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled", enabled: false},
		{name: "enabled", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newAuthTestServer(t)
			defer srv.Close()
			srv.svc.Cfg.DevLogEmailCode = tc.enabled
			cap := &captureSender{}
			srv.setEmailSender(t, cap)

			logs := &syncBuffer{}
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			resp := sendLoginCode(t, srv, "dev-log@example.com")
			require.Equal(t, http.StatusOK, resp.StatusCode)
			code := latestLoginCode(t, cap)
			out := logs.String()

			if tc.enabled {
				assert.Contains(t, out, "DEV_LOG_EMAIL_CODE")
				assert.Contains(t, out, "dev-log@example.com")
				assert.Contains(t, out, "code="+code)
			} else {
				assert.NotContains(t, out, "DEV_LOG_EMAIL_CODE")
				assert.NotContains(t, out, code)
			}
		})
	}
}
