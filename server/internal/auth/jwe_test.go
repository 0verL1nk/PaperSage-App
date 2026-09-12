package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// JWE 契约（用户需求：token 载荷不得明文泄露）：
// 紧凑串为 5 段（protected.encrypted_key.iv.ciphertext.tag），
// sub/exp/type 只存在于密文内；错误密钥与过期均拒绝。

func TestJWETokenIsEncrypted(t *testing.T) {
	sub := "11111111-2222-3333-4444-555555555555"
	access, err := CreateAccessToken("secret-1", sub, 30)
	require.NoError(t, err)

	parts := strings.Split(access, ".")
	assert.Len(t, parts, 5, "JWE 紧凑串应为 5 段")
	assert.NotContains(t, access, sub, "sub 不得明文出现")
	assert.NotContains(t, access, "access", "type 不得明文出现")
	// 注：protected header 是 base64(JSON) 也以 eyJ 开头，属 JWE 设计；
	// 明文泄露的判定是 claim（sub/type）不得出现
}

func TestJWERoundtripAndRejects(t *testing.T) {
	sub := "11111111-2222-3333-4444-555555555555"

	access, err := CreateAccessToken("secret-1", sub, 30)
	require.NoError(t, err)
	claims, err := VerifyToken(access, "secret-1", "access")
	require.NoError(t, err)
	assert.Equal(t, sub, claims.Subject)
	assert.Equal(t, "access", claims.Type)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), claims.Expiry(), time.Minute)

	// 错误密钥
	_, err = VerifyToken(access, "secret-2", "access")
	assert.Error(t, err)

	// 类型不匹配（refresh 端点拿到 access token）
	_, err = VerifyToken(access, "secret-1", "refresh")
	assert.Error(t, err)

	// 篡改密文
	tampered := access[:len(access)-3] + "AAA"
	_, err = VerifyToken(tampered, "secret-1", "access")
	assert.Error(t, err)

	// 过期
	expired, err := CreateAccessToken("secret-1", sub, -1)
	require.NoError(t, err)
	_, err = VerifyToken(expired, "secret-1", "access")
	assert.Error(t, err)
}
