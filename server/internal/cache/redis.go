package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore is a Redis-backed cache. 未调用 Connect 前所有操作安全降级
// （零值返回、无错误），对齐 Python RedisCacheStore 的语义。
type RedisStore struct {
	url    string
	client *redis.Client
}

// NewRedis creates a (not yet connected) Redis store.
func NewRedis(url string) *RedisStore {
	return &RedisStore{url: url}
}

func (r *RedisStore) Connect(context.Context) error {
	if r.client == nil {
		opts, err := redis.ParseURL(r.url) // 复用官方解析：支持密码/DB/TLS
		if err != nil {
			return err
		}
		opts.DialTimeout = 5 * time.Second
		opts.ReadTimeout = 5 * time.Second
		r.client = redis.NewClient(opts)
	}
	return nil
}

func (r *RedisStore) Disconnect(ctx context.Context) error {
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			return err
		}
		r.client = nil
	}
	return nil
}

func (r *RedisStore) Ping(ctx context.Context) bool {
	if r.client == nil {
		return false
	}
	return r.client.Ping(ctx).Err() == nil
}

func (r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	if r.client == nil {
		return "", nil
	}
	v, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (r *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if r.client == nil {
		return nil
	}
	if ttl > 0 {
		return r.client.Set(ctx, key, value, ttl).Err()
	}
	return r.client.Set(ctx, key, value, 0).Err()
}

func (r *RedisStore) Delete(ctx context.Context, keys ...string) (int, error) {
	if r.client == nil {
		return 0, nil
	}
	return int(r.client.Del(ctx, keys...).Val()), nil
}
