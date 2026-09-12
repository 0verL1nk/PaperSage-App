// Package cache 提供键值缓存抽象：Redis 优先、内存兜底。
// 行为契约对齐 backend/core/cache.py（含 refresh_token: 键前缀语义）。
package cache

import (
	"context"
	"time"
)

// Store is the key-value cache interface with TTL support.
// Get 约定：键不存在或已过期返回 ("", nil)，对应 Python 的 None。
type Store interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Ping(ctx context.Context) bool
	Get(ctx context.Context, key string) (string, error)
	// Set 写入；ttl <= 0 表示不过期（Python ex=None）。
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) (int, error)
}

// New returns a Redis-backed store when redisURL is set, else in-memory.
func New(redisURL string) Store {
	if redisURL != "" {
		return NewRedis(redisURL)
	}
	return NewMemory()
}

// RefreshTokenPrefix 与 Python 保持一致（跨实现共享 Redis 时互认）。
const RefreshTokenPrefix = "refresh_token:"

// RefreshTokenKey builds the cache key for a refresh token.
func RefreshTokenKey(token string) string {
	return RefreshTokenPrefix + token
}

// StoreRefreshToken records token → userID with TTL.
func StoreRefreshToken(ctx context.Context, s Store, token, userID string, ttl time.Duration) error {
	return s.Set(ctx, RefreshTokenKey(token), userID, ttl)
}

// RefreshTokenUser returns the userID bound to the token, if still valid.
func RefreshTokenUser(ctx context.Context, s Store, token string) (string, bool) {
	v, err := s.Get(ctx, RefreshTokenKey(token))
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// RevokeRefreshToken deletes the token; returns number of keys removed.
func RevokeRefreshToken(ctx context.Context, s Store, token string) (int, error) {
	return s.Delete(ctx, RefreshTokenKey(token))
}

// IsRefreshTokenValid reports whether the token is still tracked.
func IsRefreshTokenValid(ctx context.Context, s Store, token string) bool {
	_, ok := RefreshTokenUser(ctx, s, token)
	return ok
}
