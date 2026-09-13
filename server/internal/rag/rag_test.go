package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rag.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IndexedChunk{}))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	s, err := NewStore(db, HashEmbedder{Dim: 64}, "")
	require.NoError(t, err)
	return s
}

// Hybrid 契约：同词命中应同时被两路召回，RRF 排前面的应是最相关来源。
func TestHybridSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	doc := store.MustNewUUID()

	text := strings.Repeat("段落一：深度学习在图像识别中的应用广泛。\n\n", 5) +
		strings.Repeat("段落二：区块链技术保证数据不可篡改。\n\n", 5)
	n, err := s.IndexSource(ctx, pid, "document", doc, "survey.pdf", text)
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	hits, err := s.Search(ctx, pid, "深度学习 图像识别", 3)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "document", hits[0].SourceType)
	assert.Equal(t, "survey.pdf", hits[0].SourceName)
	assert.Contains(t, hits[0].Content, "深度学习")

	// BM25-only 语义：查询共享词越少分越低
	other, err := s.Search(ctx, pid, "量子计算 量子纠缠", 2)
	require.NoError(t, err)
	assert.Empty(t, other, "无共享词的查询不应命中")
}

// 删除来源后不再命中。
func TestDeleteSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	doc := store.MustNewUUID()
	_, err := s.IndexSource(ctx, pid, "wiki", doc, "研究笔记", "对比学习 self-supervised 表示学习范式。")
	require.NoError(t, err)

	hits, _ := s.Search(ctx, pid, "对比学习", 3)
	require.NotEmpty(t, hits)

	require.NoError(t, s.DeleteSource(ctx, pid, "wiki", doc))
	hits, _ = s.Search(ctx, pid, "对比学习", 3)
	assert.Empty(t, hits)
}

// Wiki 与 document 同库检索（sourceType 区分）。
func TestWikiAndDocumentCoexist(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	d1 := store.MustNewUUID()
	w1 := store.MustNewUUID()
	_, _ = s.IndexSource(ctx, pid, "document", d1, "transformer.pdf",
		"Transformer 架构基于自注意力机制。")
	_, _ = s.IndexSource(ctx, pid, "wiki", w1, "注意力机制笔记",
		"自注意力 self-attention 是 Transformer 的核心。")

	hits, err := s.Search(ctx, pid, "自注意力机制", 4)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hits), 2)
	types := map[string]bool{}
	for _, h := range hits {
		types[h.SourceType] = true
	}
	assert.True(t, types["document"])
	assert.True(t, types["wiki"])
}

// 分块：长文按段落切块、超长段再切句。
func TestSplitChunks(t *testing.T) {
	long := strings.Repeat("这是第一个论点的内容。", 100) + "\n\n" + strings.Repeat("第二个论点。", 200)
	chunks := SplitChunks(long, 1200, 150)
	require.Greater(t, len(chunks), 1)
	for i, c := range chunks {
		assert.Equal(t, i, c.Idx)
		assert.NotEmpty(t, c.Content)
	}
	assert.Empty(t, SplitChunks("   \n\n  ", 1200, 150))
}

// 真实嵌入端点形状（httptest 假 /embeddings）。
func TestHTTPEmbedderShape(t *testing.T) {
	// 用包内已验证逻辑：直接断言请求/响应结构
	srv := newFakeEmbedServer(t)
	e := &HTTPEmbedder{BaseURL: srv.URL, APIKey: "k", Model: "m"}
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	require.NoError(t, err)
	require.Len(t, vecs, 2)
	require.Len(t, vecs[0], 3)
	assert.InDelta(t, float64(len("a"))*0.01, vecs[0][0], 1e-6)
	_ = time.Second
}
