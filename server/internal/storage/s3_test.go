package storage

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// 用 gofakes3（进程内 S3 协议实现）测真实 aws-sdk-go-v2 客户端的
// 全套操作——覆盖 path-style、自定义 endpoint、预签名等生产路径。
func newFakeS3(t *testing.T) *S3 {
	t.Helper()
	backend := s3mem.New()
	require.NoError(t, backend.CreateBucket("papersage"))
	srv := gofakes3.New(backend)
	ts := httptest.NewServer(srv.Server())
	t.Cleanup(ts.Close)
	addr := ts.Listener.Addr().String()

	cfg := &config.Settings{
		S3Endpoint:  "http://" + addr,
		S3AccessKey: "test", S3SecretKey: "test",
		S3Bucket: "papersage", S3Region: "us-east-1",
	}
	s3, err := NewS3(context.Background(), cfg)
	require.NoError(t, err)
	return s3
}

func TestS3PutGetDeleteRoundtrip(t *testing.T) {
	s3 := newFakeS3(t)
	ctx := context.Background()

	require.NoError(t, s3.Put(ctx, "projects/p1/latex/main.tex", []byte("hello latex")))

	got, err := s3.Get(ctx, "projects/p1/latex/main.tex")
	require.NoError(t, err)
	assert.Equal(t, "hello latex", string(got))

	assert.True(t, s3.Exists(ctx, "projects/p1/latex/main.tex"))
	assert.False(t, s3.Exists(ctx, "projects/p1/latex/missing.tex"))

	require.NoError(t, s3.Delete(ctx, "projects/p1/latex/main.tex"))
	assert.False(t, s3.Exists(ctx, "projects/p1/latex/main.tex"))

	_, err = s3.Get(ctx, "projects/p1/latex/main.tex")
	assert.Error(t, err)
}

func TestS3ListPrefix(t *testing.T) {
	s3 := newFakeS3(t)
	ctx := context.Background()
	for _, k := range []string{"p/a.tex", "p/b.tex", "q/c.tex"} {
		require.NoError(t, s3.Put(ctx, k, []byte("x")))
	}

	keys, err := s3.List(ctx, "p/")
	require.NoError(t, err)
	assert.Equal(t, []string{"p/a.tex", "p/b.tex"}, keys, "仅列前缀下对象且字典序")

	all, err := s3.List(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestS3PresignGet(t *testing.T) {
	s3 := newFakeS3(t)
	require.NoError(t, s3.Put(context.Background(), "k", []byte("v")))

	url, err := s3.PresignGet(context.Background(), "k", 5*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, url, "X-Amz-Signature", "预签名 URL 应带签名参数")
	assert.Contains(t, url, "/papersage/k")
}

// BuildKey 契约：projects/{id}/latex/{path}
func TestBuildKey(t *testing.T) {
	assert.Equal(t, "projects/abc/latex/main.tex", BuildKey("abc", "main.tex"))
	assert.Equal(t, "projects/abc/latex/sec/intro.tex", BuildKey("abc", "sec/intro.tex"))
}
