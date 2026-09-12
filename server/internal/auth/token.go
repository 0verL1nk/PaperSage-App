package auth

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwe"
	"golang.org/x/crypto/hkdf"
	"io"
)

// Token 采用 JWE（A256KW + A256GCM）：载荷加密传输，sub/exp/type
// 不再像 JWT 那样 base64 明文可读（用户要求避免明文泄露）。
// 密钥：SECRET_KEY（任意长度字符串）经 HKDF-SHA256 派生 32 字节 KEK。

type Claims struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"` // unix 秒
	Type      string `json:"type"`
}

func (c *Claims) Expiry() time.Time { return time.Unix(c.ExpiresAt, 0) }

// CreateAccessToken issues an encrypted access token.
func CreateAccessToken(secret, sub string, expireMinutes int) (string, error) {
	return encryptClaims(secret, sub, "access", time.Duration(expireMinutes)*time.Minute)
}

// CreateRefreshToken issues an encrypted refresh token.
func CreateRefreshToken(secret, sub string, expireDays int) (string, error) {
	return encryptClaims(secret, sub, "refresh", time.Duration(expireDays)*24*time.Hour)
}

func encryptClaims(secret, sub, typ string, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(Claims{
		Subject:   sub,
		ExpiresAt: time.Now().Add(ttl).Unix(),
		Type:      typ,
	})
	if err != nil {
		return "", err
	}
	kek, err := deriveKEK(secret)
	if err != nil {
		return "", err
	}
	compact, err := jwe.Encrypt(payload,
		jwe.WithKey(jwa.A256KW, kek),
		jwe.WithContentEncryption(jwa.A256GCM),
	)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

// VerifyToken decrypts and checks expiry plus the expected token type.
func VerifyToken(tokenStr, secret, tokenType string) (*Claims, error) {
	kek, err := deriveKEK(secret)
	if err != nil {
		return nil, err
	}
	plaintext, err := jwe.Decrypt([]byte(tokenStr), jwe.WithKey(jwa.A256KW, kek))
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(plaintext, &claims); err != nil {
		return nil, errors.New("invalid claims")
	}
	if claims.ExpiresAt == 0 || time.Now().After(claims.Expiry()) {
		return nil, errors.New("token is expired")
	}
	if claims.Type != tokenType {
		return nil, errors.New("invalid token type")
	}
	return &claims, nil
}

// deriveKEK 从任意长度的 SECRET_KEY 派生 32 字节 KEK（A256KW 要求）。
func deriveKEK(secret string) ([]byte, error) {
	hk := hkdf.New(sha256.New, []byte(secret), []byte("papersage-jwe-v1"), []byte("kek"))
	kek := make([]byte, 32)
	if _, err := io.ReadFull(hk, kek); err != nil {
		return nil, err
	}
	return kek, nil
}
