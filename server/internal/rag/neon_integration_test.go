package rag

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Uses only a session-local temporary table; the caller must supply an isolated
// test PostgreSQL instance with pgvector already installed.
func newIsolatedPG(t *testing.T) *PgStore {
	t.Helper()
	dsn := os.Getenv("RAG_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("RAG_TEST_PG_DSN not set")
	}
	ctx := context.Background()
	pg, err := NewPgStore(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Close(context.Background()) })
	_, err = pg.conn.Exec(ctx, `BEGIN`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pg.conn.Exec(context.Background(), `ROLLBACK`) })
	// Keep PgBouncer transaction pooling pinned to one backend through cleanup.
	require.Equal(t, byte('T'), pg.conn.PgConn().TxStatus())
	_, err = pg.conn.Exec(ctx, `CREATE TEMP TABLE rag_chunks (
		id varchar(80) PRIMARY KEY, project_id varchar(32) NOT NULL,
		source_type varchar(20) NOT NULL, source_id varchar(32) NOT NULL,
		source_name varchar(255) NOT NULL, chunk_idx int NOT NULL,
		content text NOT NULL, embedding vector(1024), generation bigint NOT NULL
	)`)
	require.NoError(t, err)
	_, err = pg.conn.Exec(ctx, `SET search_path = pg_temp, public`)
	require.NoError(t, err)
	return pg
}

// pgvector integration uses only the explicit isolated test database.
func TestNeonPgVectorIntegration(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(nil, HashEmbedder{Dim: 1024}, "")
	require.NoError(t, err)
	s.Pg = newIsolatedPG(t)

	// 索引两个来源（HashEmbedder 词袋向量：共享词 → 相似）
	_, nErr := s.IndexSource(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"document", storeUUIDFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), "deep.pdf",
		"深度学习 图像识别\n\n卷积神经网络 深度学习 应用")
	require.NoError(t, nErr)
	_, nErr = s.IndexSource(ctx, storeUUIDFromHex("11111111111111111111111111111111"),
		"document", storeUUIDFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), "block.pdf",
		"区块链 共识机制\n\n分布式账本 区块链 应用")
	require.NoError(t, nErr)

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

type pgTestEmbedder struct{}

func (pgTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 1024)
		out[i][0] = 1
	}
	return out, nil
}

func (pgTestEmbedder) Describe() (string, int) { return "pg-test", 1024 }

func TestIsolatedPGHybridSearch(t *testing.T) {
	ctx := context.Background()
	pg := newIsolatedPG(t)
	var backendPID int
	require.NoError(t, pg.conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&backendPID))
	var err error

	pid, otherPID := store.MustNewUUID(), store.MustNewUUID()
	bmID, vecID, otherID := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	if bmID.String() > vecID.String() {
		bmID, vecID = vecID, bmID
	}
	vec := make([]float32, 1024)
	vec[0] = 1
	orthogonal := make([]float32, 1024)
	orthogonal[1] = 1
	require.NoError(t, pg.IndexSource(ctx, hexID(pid), "document", hexID(vecID), "vector.pdf", []Chunk{{Content: "unrelated"}}, [][]float32{vec}, 1))
	require.NoError(t, pg.IndexSource(ctx, hexID(pid), "document", hexID(bmID), "lexical.pdf", []Chunk{{Content: "quokka"}}, [][]float32{orthogonal}, 2))
	require.NoError(t, pg.IndexSource(ctx, hexID(otherPID), "document", hexID(otherID), "other.pdf", []Chunk{{Content: "quokka"}}, [][]float32{vec}, 2))

	rows, err := pg.SearchVector(ctx, hexID(pid), vec, 5, 2)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "lexical.pdf", rows[0].SourceName)
	_, err = pg.conn.Exec(ctx, `SAVEPOINT invalid_vector`)
	require.NoError(t, err)
	_, err = pg.SearchVector(ctx, hexID(pid), []float32{1}, 5, 2)
	require.Error(t, err)
	_, err = pg.conn.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_vector`)
	require.NoError(t, err)

	s, err := NewStore(nil, pgTestEmbedder{}, "")
	require.NoError(t, err)
	s.Pg = pg
	hits, err := s.Search(ctx, pid, "quokka", 1)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "lexical.pdf", hits[0].SourceName)
	assert.Equal(t, hexID(bmID), hits[0].SourceID)
	assert.Equal(t, "quokka", hits[0].Content)

	rows, err = pg.RowsByID(ctx, hexID(pid), 1, []string{hits[0].ChunkID})
	require.NoError(t, err)
	assert.Empty(t, rows, "other generations must not be returned")
	rows, err = pg.RowsByID(ctx, hexID(otherPID), 0, []string{hits[0].ChunkID})
	require.NoError(t, err)
	assert.Empty(t, rows, "other projects must not be returned")
	var finalBackendPID int
	require.NoError(t, pg.conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&finalBackendPID))
	assert.Equal(t, backendPID, finalBackendPID, "all test queries must use the same backend")
	assert.Equal(t, byte('T'), pg.conn.PgConn().TxStatus())
}

func TestIsolatedPGConcurrentSearch(t *testing.T) {
	pg := newIsolatedPG(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pid, sourceID := store.MustNewUUID(), store.MustNewUUID()
	vec := make([]float32, 1024)
	vec[0] = 1
	require.NoError(t, pg.IndexSource(ctx, hexID(pid), "document", hexID(sourceID), "doc.pdf", []Chunk{{Content: "quokka"}}, [][]float32{vec}, 1))
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		<-start
		for i := 0; i < 20; i++ {
			rows, err := pg.SearchVector(ctx, hexID(pid), vec, 1, 1)
			if err != nil {
				done <- err
				return
			}
			if len(rows) != 1 || rows[0].Content != "quokka" {
				done <- fmt.Errorf("unexpected vector rows: %v", rows)
				return
			}
		}
		done <- nil
	}()
	go func() {
		<-start
		for i := 0; i < 20; i++ {
			rows, err := pg.SearchBM25(ctx, hexID(pid), 1)
			if err != nil {
				done <- err
				return
			}
			if len(rows) != 1 || rows[0].Content != "quokka" {
				done <- fmt.Errorf("unexpected BM25 rows: %v", rows)
				return
			}
		}
		done <- nil
	}()
	close(start)
	first, second := <-done, <-done
	assert.NoError(t, first)
	assert.NoError(t, second)
}

func storeUUIDFromHex(hex string) store.UUID {
	u, err := uuid.Parse(hex)
	if err != nil {
		panic(err)
	}
	return store.UUID{UUID: u}
}
