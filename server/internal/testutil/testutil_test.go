package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试基座自身的单元测试：基座 bug 会静默污染全部依赖它的契约测试。

func TestMemStorageAllOperations(t *testing.T) {
	m := NewMemStorage()
	ctx := context.Background()

	// 初始态：Get miss、Exists false
	_, err := m.Get(ctx, "k")
	require.Error(t, err, "未写入的 key 应返回错误")
	assert.False(t, m.Exists(ctx, "k"))

	// Put/Get roundtrip
	require.NoError(t, m.Put(ctx, "k", []byte("v1")))
	got, err := m.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got))
	assert.True(t, m.Exists(ctx, "k"))

	// List 前缀（字典序）
	require.NoError(t, m.Put(ctx, "p/a", []byte("1")))
	require.NoError(t, m.Put(ctx, "p/b", []byte("2")))
	keys, err := m.List(ctx, "p/")
	require.NoError(t, err)
	assert.Equal(t, []string{"p/a", "p/b"}, keys)

	// Presign
	url, err := m.PresignGet(ctx, "k", time.Minute)
	require.NoError(t, err)
	assert.Contains(t, url, "k")

	// Delete 后不存在
	require.NoError(t, m.Delete(ctx, "p/a"))
	assert.False(t, m.Exists(ctx, "p/a"))
}

func TestTestServerDoSendsAuthAndBody(t *testing.T) {
	ts := New(t)

	// Do 带 Bearer token → 认证端点通过
	resp := ts.Do(t, http.MethodGet, "/api/v1/settings", nil)
	require.Equal(t, 200, resp.StatusCode, "带 token 的请求应通过认证")

	// Do 携带 JSON body 创建项目
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/", []byte(`{"name":"do-test"}`))
	require.Equal(t, 200, resp.StatusCode, "JSON body 创建项目")
}
