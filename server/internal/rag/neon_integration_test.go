package rag

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Neon(pgvector) 集成：NEON_DATABASE_URL 存在才跑（真库验证 HNSW 下推）。
func TestNeonPgVectorIntegration(t *testing.T) {
	dsn := os.Getenv("NEON_DATABASE_URL")
	if dsn == "" {
		t.Skip("NEON_DATABASE_URL not set")
	}
	ctx := context.Background()

	s, err := NewStore(nil, HashEmbedder{Dim: 1024}, "")
	require.NoError(t, err)
	require.NoError(t, s.AttachNeon(ctx, dsn))
	t.Cleanup(func() { _ = s.Pg.Close(ctx) })

	// 清残留
	require.NoError(t, s.Pg.DeleteSource(ctx, "11111111111111111111111111111111", "document", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))

	// 索引两个来源（HashEmbedder 词袋向量：共享词 → 相似）
	_, nErr := s.IndexSource(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"document", storeUUIDFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), "deep.pdf",
		"深度学习 图像识别\n\n卷积神经网络 深度学习 应用")
	require.NoError(t, nErr)
	_, nErr = s.IndexSource(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"document", storeUUIDFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), "block.pdf",
		"区块链 共识机制\n\n分布式账本 区块链 应用")
	require.NoError(t, nErr)
	t.Cleanup(func() {
		_ = s.Pg.DeleteSource(ctx, "11111111111111111111111111111111", "document", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		_ = s.Pg.DeleteSource(ctx, "11111111111111111111111111111111", "document", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	})

	// 向量检索下推 PG：相关查询命中对应文档
	hits, err := s.Search(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"深度学习 图像识别", 2)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "deep.pdf", hits[0].SourceName, "最相关应为 deep.pdf")
	// 哈希嵌入有维度碰撞噪声——只断言排序（deep 必须第一），
	// 语义无关性由真实嵌入模型保证

	// BM25 路也走 Neon 行源
	hits2, err := s.Search(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"区块链", 2)
	require.NoError(t, err)
	require.NotEmpty(t, hits2)
	assert.Equal(t, "block.pdf", hits2[0].SourceName)
}

func storeUUIDFromHex(hex string) store.UUID {
	u, err := uuid.Parse(hex)
	if err != nil {
		panic(err)
	}
	return store.UUID{UUID: u}
}
