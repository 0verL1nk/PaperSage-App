package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Python bcrypt 生成的哈希必须能在 Go 侧验证（存量用户可直接登录）
func TestPythonBcryptHashVerifiesInGo(t *testing.T) {
	f := loadFixtures(t)

	assert.True(t, VerifyPassword(f.Sha256PW, f.BcryptHash), "Python 生成的哈希必须验证通过")
	assert.False(t, VerifyPassword("wrong-sha256", f.BcryptHash))
}

// Go 生成的哈希可往返验证（bcrypt 跨实现天然兼容，$2b$ 前缀一致）
func TestHashPasswordRoundtrip(t *testing.T) {
	h, err := HashPassword("abc123-sha256hex")
	require.NoError(t, err)
	assert.NotEqual(t, "abc123-sha256hex", h)
	assert.True(t, VerifyPassword("abc123-sha256hex", h))
}

// 非法哈希串不 panic，返回 false（对应 Python 的 try/except）
func TestVerifyPasswordMalformedHash(t *testing.T) {
	assert.False(t, VerifyPassword("x", "not-a-bcrypt-hash"))
}
