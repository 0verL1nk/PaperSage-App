package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// 契约来源：backend/core/security.py — bcrypt 哈希/校验，
// 前端已 SHA-256，这里仅 bcrypt 层（双重哈希的第三层不变）。

// HashPassword hashes with bcrypt（$2b$ 前缀，与 Python bcrypt 库一致）。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword returns false（而非 error）on mismatch or malformed hash.
func VerifyPassword(plain, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}
