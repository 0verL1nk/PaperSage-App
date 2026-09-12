package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：backend/core/cache.py

func TestMemoryRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.Connect(ctx))

	require.NoError(t, s.Set(ctx, "k", "v", 0))
	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestMemoryTTLExpiry(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.Connect(ctx))

	require.NoError(t, s.Set(ctx, "k", "v", 50*time.Millisecond))
	time.Sleep(60 * time.Millisecond)

	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "", got, "过期后应返回空串（Python None）")
}

func TestMemoryDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.Connect(ctx))
	require.NoError(t, s.Set(ctx, "a", "1", 0))
	require.NoError(t, s.Set(ctx, "b", "2", 0))

	n, err := s.Delete(ctx, "a", "missing", "b")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestMemoryDisconnectClears(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.Connect(ctx))
	require.NoError(t, s.Set(ctx, "k", "v", 0))
	require.NoError(t, s.Disconnect(ctx))

	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestMemoryPing(t *testing.T) {
	assert.True(t, NewMemory().Ping(context.Background()))
}

// Redis 实现：miniredis 假实例，语义与 redis-py 行为对齐
func newRedisTestStore(t *testing.T) (context.Context, *RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	s := NewRedis("redis://" + mr.Addr())
	require.NoError(t, s.Connect(context.Background()))
	return context.Background(), s, mr
}

func TestRedisRoundtrip(t *testing.T) {
	ctx, s, _ := newRedisTestStore(t)

	require.NoError(t, s.Set(ctx, "k", "v", 0))
	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	n, err := s.Delete(ctx, "k", "nope")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestRedisTTLExpiry(t *testing.T) {
	ctx, s, mr := newRedisTestStore(t)

	require.NoError(t, s.Set(ctx, "k", "v", time.Minute))
	mr.FastForward(2 * time.Minute)

	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

// 未 connect 时各操作安全返回零值（Python RedisCacheStore 语义）
func TestRedisUnconnectedDegrades(t *testing.T) {
	s := NewRedis("redis://127.0.0.1:1/0") // 不调用 Connect
	ctx := context.Background()

	got, err := s.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "", got)

	require.NoError(t, s.Set(ctx, "k", "v", 0))
	n, err := s.Delete(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, s.Ping(ctx))
}

// 工厂：REDIS_URL 有值 → Redis；否则内存
func TestFactory(t *testing.T) {
	assert.IsType(t, &RedisStore{}, New("redis://localhost:6379/0"))
	assert.IsType(t, &MemoryStore{}, New(""))
}

// Refresh token 辅助：前缀 refresh_token:，存取/吊销/校验
func TestRefreshTokenHelpers(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	require.NoError(t, s.Connect(ctx))

	require.NoError(t, StoreRefreshToken(ctx, s, "tok-1", "user-9", time.Hour))

	uid, ok := RefreshTokenUser(ctx, s, "tok-1")
	require.True(t, ok)
	assert.Equal(t, "user-9", uid)
	assert.True(t, IsRefreshTokenValid(ctx, s, "tok-1"))

	n, err := RevokeRefreshToken(ctx, s, "tok-1")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, ok = RefreshTokenUser(ctx, s, "tok-1")
	assert.False(t, ok, "吊销后应无效（Python is_refresh_token_valid → False）")
	assert.False(t, IsRefreshTokenValid(ctx, s, "tok-1"))
}

// 前缀格式必须与 Python 一致（跨实现共享缓存时互认）
func TestRefreshTokenKeyFormat(t *testing.T) {
	assert.Equal(t, "refresh_token:abc", RefreshTokenKey("abc"))
}
