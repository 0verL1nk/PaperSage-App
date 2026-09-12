package auth

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 跨实现契约：Python（python-jose/bcrypt）产出的固件在 Go 侧必须可验签（design D4）
type pythonFixtures struct {
	Secret         string `json:"secret"`
	AccessValid    string `json:"access_valid"`
	RefreshValid   string `json:"refresh_valid"`
	AccessExpired  string `json:"access_expired"`
	AccessWrongTyp string `json:"access_wrong_type"`
	AccessBadSig   string `json:"access_bad_sig"`
	BcryptHash     string `json:"python_bcrypt_hash"`
	Sha256PW       string `json:"python_sha256_pw"`
}

func loadFixtures(t *testing.T) pythonFixtures {
	t.Helper()
	b, err := os.ReadFile("testdata/python_fixtures.json")
	require.NoError(t, err)
	var f pythonFixtures
	require.NoError(t, json.Unmarshal(b, &f))
	return f
}

// Python 签发的 token 在 Go 侧验证通过（存量 token 兼容的核心保证）
// TestPythonTokensVerifyInGo 已删除：token 已 JWE 化（用户要求避免明文
// 泄露），与 python-jose 的互验兼容随之有意放弃（python 后端已删除）。

func TestPythonTokenRejections(t *testing.T) {
	f := loadFixtures(t)

	_, err := VerifyToken(f.AccessExpired, f.Secret, "access")
	assert.Error(t, err, "过期 token 应被拒绝")

	_, err = VerifyToken(f.AccessWrongTyp, f.Secret, "access")
	assert.Error(t, err, "type 不匹配应被拒绝")

	_, err = VerifyToken(f.AccessBadSig, f.Secret, "access")
	assert.Error(t, err, "错误签名应被拒绝")
}

// Go 签发的 token 自洽（exp/type claims 与 Python 语义一致）
func TestGoTokenRoundtrip(t *testing.T) {
	sub := "22222222-2222-2222-2222-222222222222"

	access, err := CreateAccessToken("s3cret", sub, 30)
	require.NoError(t, err)
	claims, err := VerifyToken(access, "s3cret", "access")
	require.NoError(t, err)
	assert.Equal(t, sub, claims.Subject)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), claims.Expiry(), time.Minute)

	refresh, err := CreateRefreshToken("s3cret", sub, 7)
	require.NoError(t, err)
	_, err = VerifyToken(refresh, "s3cret", "access")
	assert.Error(t, err, "refresh token 不能当 access 用")
}
