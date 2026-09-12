package cache

import (
	"context"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

// MemoryStore 基于 go-cache 的内存实现（并发安全、TTL、惰性过期 + 定期清理），
// 对应 backend/core/cache.py 的 MemoryCacheStore。
type MemoryStore struct {
	c *gocache.Cache
}

// NewMemory creates an in-memory store（默认不过期，5 分钟清理一次过期项）。
func NewMemory() *MemoryStore {
	return &MemoryStore{c: gocache.New(gocache.NoExpiration, 5*time.Minute)}
}

func (m *MemoryStore) Connect(context.Context) error { return nil }

func (m *MemoryStore) Disconnect(context.Context) error {
	m.c.Flush()
	return nil
}

func (m *MemoryStore) Ping(context.Context) bool { return true }

func (m *MemoryStore) Get(_ context.Context, key string) (string, error) {
	v, found := m.c.Get(key)
	if !found {
		return "", nil // 对应 Python None
	}
	s, _ := v.(string)
	return s, nil
}

func (m *MemoryStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	d := gocache.NoExpiration
	if ttl > 0 {
		d = ttl
	}
	m.c.Set(key, value, d)
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, keys ...string) (int, error) {
	n := 0
	for _, k := range keys {
		if _, found := m.c.Get(k); found {
			m.c.Delete(k)
			n++
		}
	}
	return n, nil
}
