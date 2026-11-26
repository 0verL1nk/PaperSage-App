package netguard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBaseURLRejectsUnsafeTargets(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":             "",
		"no scheme":         "api.example.com/v1",
		"ftp scheme":        "ftp://api.example.com/v1",
		"file scheme":       "file:///etc/passwd",
		"no host":           "http:///v1",
		"credentials":       "https://user:pass@api.example.com/v1",
		"loopback v4":       "http://127.0.0.1:11434/v1",
		"private v4":        "http://10.0.0.5/v1",
		"link local":        "http://169.254.169.254/latest/meta-data",
		"cgnat":             "http://100.64.0.1/v1",
		"loopback v6":       "http://[::1]:11434/v1",
		"unspecified":       "http://0.0.0.0:8080/v1",
		"ipv6 unique local": "http://[fd00::1]/v1",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateBaseURL(raw, false)
			require.Error(t, err, raw)
		})
	}
}

func TestValidateBaseURLAllowsPublicAndTrimsTrailingSlash(t *testing.T) {
	base, err := ValidateBaseURL("  https://api.example.com/v1/  ", false)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1", base)

	// 公网字面量 IP 直接放行，域名交给拨号期检查。
	_, err = ValidateBaseURL("http://8.8.8.8/v1", false)
	require.NoError(t, err)

	// 开发开关：显式允许本地模型。
	base, err = ValidateBaseURL("http://127.0.0.1:11434/v1/", true)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:11434/v1", base)
}

func TestControlBlocksNonPublicAddresses(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:443", "10.0.0.5:80", "192.168.1.2:80",
		"169.254.169.254:80", "100.64.0.1:80", "[::1]:80", "0.0.0.0:80",
		"[fd00::1]:80", "224.0.0.1:80",
	} {
		require.Error(t, Control(false)("tcp", address, nil), address)
	}
	require.NoError(t, Control(false)("tcp", "8.8.8.8:443", nil))
	require.Error(t, Control(false)("tcp", "api.example.com:443", nil), "未解析到 IP 的地址不能建立连接")
}

func TestControlAllowPrivateLetsLocalModelsThrough(t *testing.T) {
	for _, address := range []string{"127.0.0.1:11434", "10.0.0.5:80", "[::1]:11434"} {
		require.NoError(t, Control(true)("tcp", address, nil), address)
	}
}
