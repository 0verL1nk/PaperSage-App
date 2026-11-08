package rag

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

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
	require.NoError(t, db.AutoMigrate(&IndexedChunk{}, &RagIndexGeneration{}))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	s, err := NewStore(db, HashEmbedder{Dim: 4096}, "")
	require.NoError(t, err)
	return s
}

type failingQueryEmbedder struct{ err error }

func (e failingQueryEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, e.err
}

func (failingQueryEmbedder) Describe() (string, int) { return "failing", 0 }

func TestSearchRejectsMissingQueryVector(t *testing.T) {
	s := newTestStore(t)
	s.Embed = failingQueryEmbedder{}
	_, err := s.Search(context.Background(), store.MustNewUUID(), "query", 5)
	require.ErrorContains(t, err, "expected one nonempty vector")
}

func TestSearchReportsQueryEmbeddingFailure(t *testing.T) {
	s := newTestStore(t)
	want := errors.New("embedding unavailable")
	s.Embed = failingQueryEmbedder{err: want}
	_, err := s.Search(context.Background(), store.MustNewUUID(), "query", 5)
	require.ErrorIs(t, err, want)
}

func TestSearchRetriesFailedBM25Load(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	_, err := s.ActiveGeneration(ctx, pid)
	require.NoError(t, err)
	// Warm only the vector index so the next search must load BM25 from the DB.
	_, err = s.vectorIndex(ctx, pid, 1)
	require.NoError(t, err)
	require.NoError(t, s.DB.Migrator().DropTable(&IndexedChunk{}))
	_, err = s.Search(ctx, pid, "quokka", 5)
	require.Error(t, err)

	require.NoError(t, s.DB.AutoMigrate(&IndexedChunk{}))
	require.NoError(t, s.DB.Create(&IndexedChunk{
		ID: "bm-only", ProjectID: pid, SourceID: store.MustNewUUID(),
		SourceType: "document", SourceName: "test.pdf", Content: "quokka", Generation: 1,
	}).Error)
	hits, err := s.Search(ctx, pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "bm-only", hits[0].ChunkID)
}

func TestSearchRetriesFailedVectorLoad(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	_, err := s.ActiveGeneration(ctx, pid)
	require.NoError(t, err)
	require.NoError(t, s.DB.Migrator().DropTable(&IndexedChunk{}))
	_, err = s.Search(ctx, pid, "quokka", 5)
	require.Error(t, err)

	require.NoError(t, s.DB.AutoMigrate(&IndexedChunk{}))
	vecs, err := s.Embed.Embed(ctx, []string{"quokka"})
	require.NoError(t, err)
	require.NoError(t, s.DB.Create(&IndexedChunk{
		ID: "vector", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "unrelated",
		Embedding: EncodeVec(vecs[0]), Generation: 1,
	}).Error)
	hits, err := s.Search(ctx, pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
}

type cancelQueryEmbedder struct{ cancel context.CancelFunc }

func (e cancelQueryEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	e.cancel()
	return [][]float32{{1}}, nil
}

func (cancelQueryEmbedder) Describe() (string, int) { return "cancel", 1 }

func TestSearchReportsCancellationWithWarmCaches(t *testing.T) {
	s := newTestStore(t)
	pid := store.MustNewUUID()
	_, err := s.Search(context.Background(), pid, "query", 5)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	s.Embed = cancelQueryEmbedder{cancel: cancel}
	_, err = s.Search(ctx, pid, "query", 5)
	require.ErrorIs(t, err, context.Canceled)
}

func TestBM25LoadRespectsCancellation(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.bm25For(ctx, store.MustNewUUID(), 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSearchReportsBM25CandidateQueryFailure(t *testing.T) {
	s := newTestStore(t)
	pid := store.MustNewUUID()
	row := IndexedChunk{ID: "bm-only", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "quokka", Generation: 1}
	require.NoError(t, s.DB.Create(&row).Error)
	_, err := s.Search(context.Background(), pid, "quokka", 5)
	require.NoError(t, err)
	require.NoError(t, s.DB.Migrator().DropTable(&IndexedChunk{}))
	_, err = s.Search(context.Background(), pid, "quokka", 5)
	require.ErrorContains(t, err, "load BM25 candidates")
}

func TestSearchReportsMissingBM25Candidate(t *testing.T) {
	s := newTestStore(t)
	pid := store.MustNewUUID()
	row := IndexedChunk{ID: "bm-only", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "quokka", Generation: 1}
	require.NoError(t, s.DB.Create(&row).Error)
	hits, err := s.Search(context.Background(), pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.NoError(t, s.DB.Delete(&row).Error)
	_, err = s.Search(context.Background(), pid, "quokka", 5)
	require.ErrorContains(t, err, "BM25 candidate")
	row.ProjectID = store.MustNewUUID()
	require.NoError(t, s.DB.Create(&row).Error)
	_, err = s.Search(context.Background(), pid, "quokka", 5)
	require.ErrorContains(t, err, "BM25 candidate")
	require.NoError(t, s.DB.Model(&row).Updates(map[string]any{"project_id": pid, "generation": 2}).Error)
	_, err = s.Search(context.Background(), pid, "quokka", 5)
	require.ErrorContains(t, err, "BM25 candidate")
}

func TestSearchReloadsCachesForChangedGeneration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()
	_, err := s.ActiveGeneration(ctx, pid)
	require.NoError(t, err)
	vecs, err := s.Embed.Embed(ctx, []string{"quokka"})
	require.NoError(t, err)
	rows := []IndexedChunk{
		{ID: "old", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "quokka", Embedding: EncodeVec(vecs[0]), Generation: 1},
		{ID: "new", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "quokka", Embedding: EncodeVec(vecs[0]), Generation: 2},
	}
	require.NoError(t, s.DB.Create(&rows).Error)
	hits, err := s.Search(ctx, pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "old", hits[0].ChunkID)
	require.NoError(t, s.DB.Model(&RagIndexGeneration{}).Where("project_id = ? AND status = ?", pid, GenActive).Update("generation", 2).Error)
	hits, err = s.Search(ctx, pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "new", hits[0].ChunkID)
}

func TestIndexLoadDoesNotRepopulateAfterInvalidation(t *testing.T) {
	for _, kind := range []string{"vector", "bm25"} {
		for _, cause := range []string{"invalidate", "cancel"} {
			t.Run(kind+"/"+cause, func(t *testing.T) {
				s := newTestStore(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				pid := store.MustNewUUID()
				row := IndexedChunk{ID: "old", ProjectID: pid, SourceID: store.MustNewUUID(), Content: "quokka", Embedding: EncodeVec([]float32{1}), Generation: 1}
				require.NoError(t, s.DB.Create(&row).Error)
				loaded := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				defer releaseOnce.Do(func() { close(release) })
				require.NoError(t, s.DB.Callback().Query().After("gorm:query").Register("test:block-loaded-index", func(tx *gorm.DB) {
					if tx.Statement.Table == "rag_chunks" {
						close(loaded)
						<-release
					}
				}))
				done := make(chan error, 1)
				go func() {
					if kind == "vector" {
						_, err := s.vectorIndex(ctx, pid, 1)
						done <- err
					} else {
						_, err := s.bm25For(ctx, pid, 1)
						done <- err
					}
				}()
				select {
				case <-loaded:
				case <-time.After(5 * time.Second):
					t.Fatal("index query did not reach the barrier")
				}
				if cause == "invalidate" {
					require.NoError(t, s.DB.Delete(&row).Error)
					s.invalidate(pid)
				} else {
					cancel()
				}
				releaseOnce.Do(func() { close(release) })
				err := <-done
				require.Error(t, err)
				if cause == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				require.NoError(t, s.DB.Callback().Query().Remove("test:block-loaded-index"))
				if kind == "vector" {
					idx, err := s.vectorIndex(context.Background(), pid, 1)
					require.NoError(t, err)
					if cause == "invalidate" {
						assert.Empty(t, idx.entries)
					} else {
						assert.Len(t, idx.entries, 1)
					}
				} else {
					idx, err := s.bm25For(context.Background(), pid, 1)
					require.NoError(t, err)
					if cause == "invalidate" {
						assert.Empty(t, idx.docs)
					} else {
						assert.Equal(t, []string{"old"}, idx.docs)
					}
				}
			})
		}
	}
}

func TestPgStoreGateCancellationAndRelease(t *testing.T) {
	pg := &PgStore{gate: make(chan struct{}, 1)}
	require.NoError(t, pg.acquire(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pg.acquire(ctx) }()
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	pg.release()
	require.NoError(t, pg.acquire(context.Background()))
	pg.release()
}

func TestPgStoreMethodsCancelWhileWaiting(t *testing.T) {
	pg := &PgStore{gate: make(chan struct{}, 1)}
	require.NoError(t, pg.acquire(context.Background()))
	defer pg.release()
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"index", func(ctx context.Context) error { return pg.IndexSource(ctx, "p", "document", "s", "n", nil, nil, 1) }},
		{"delete", func(ctx context.Context) error { return pg.DeleteSource(ctx, "p", "document", "s") }},
		{"vector", func(ctx context.Context) error { _, err := pg.SearchVector(ctx, "p", []float32{1}, 1, 1); return err }},
		{"bm25", func(ctx context.Context) error { _, err := pg.SearchBM25(ctx, "p", 1); return err }},
		{"rows", func(ctx context.Context) error { _, err := pg.RowsByID(ctx, "p", 1, []string{"id"}); return err }},
		{"close", pg.Close},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			started := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				close(started)
				done <- tc.call(ctx)
			}()
			<-started
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not stop waiting after cancellation")
			}
		})
	}
}

func TestPgStoreGateSerializesAccess(t *testing.T) {
	pg := &PgStore{gate: make(chan struct{}, 1)}
	const workers, iterations = 8, 20
	var wg sync.WaitGroup
	var active, total atomic.Int32
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := pg.acquire(context.Background()); err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				if active.Add(1) != 1 {
					t.Error("gate allowed simultaneous access")
				}
				runtime.Gosched()
				total.Add(1)
				active.Add(-1)
				pg.release()
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, workers*iterations, total.Load())
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

func TestSplitChunksUTF8Boundaries(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		target int
		want   []string
	}{
		{"chinese", "中文测试", 5, []string{"中", "文", "测", "试"}},
		{"mixed", "ab😀cd", 5, []string{"ab", "😀c", "d"}},
		{"tiny budget", "中😀a", 1, []string{"中", "😀", "a"}},
		{"sentence boundary", "ab。中文测试", 9, []string{"ab。", "中文测", "试"}},
		{"ascii", "abcdefgh", 3, []string{"abc", "def", "gh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// overlap is currently ignored; both inputs must retain the same order.
			for _, overlap := range []int{0, 2} {
				chunks := SplitChunks(tc.text, tc.target, overlap)
				for i, chunk := range chunks {
					assert.True(t, utf8.ValidString(chunk.Content), "chunk %d is invalid UTF-8: %q", i, chunk.Content)
				}
				require.Len(t, chunks, len(tc.want))
				for i, chunk := range chunks {
					assert.Equal(t, i, chunk.Idx)
					assert.Equal(t, tc.want[i], chunk.Content)
				}
			}
		})
	}
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

func TestIndexSourceKeepsOldRowsWhenEmbeddingFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid, sid := store.MustNewUUID(), store.MustNewUUID()
	_, err := s.IndexSource(ctx, pid, "document", sid, "doc.txt", "old content")
	require.NoError(t, err)

	s.Embed = failingQueryEmbedder{err: errors.New("embed down")}
	_, err = s.IndexSource(ctx, pid, "document", sid, "doc.txt", "new content")
	require.Error(t, err)

	var rows []IndexedChunk
	require.NoError(t, s.DB.Where("project_id = ? AND source_type = ? AND source_id = ?", pid, "document", sid).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Contains(t, rows[0].Content, "old content")
}

func TestIndexSourceClearsRowsForEmptyText(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid, sid := store.MustNewUUID(), store.MustNewUUID()
	_, err := s.IndexSource(ctx, pid, "document", sid, "doc.txt", "old content")
	require.NoError(t, err)

	_, err = s.IndexSource(ctx, pid, "document", sid, "doc.txt", "   \n\n  ")
	require.NoError(t, err)

	var count int64
	require.NoError(t, s.DB.Model(&IndexedChunk{}).
		Where("project_id = ? AND source_type = ? AND source_id = ?", pid, "document", sid).Count(&count).Error)
	require.Zero(t, count)
}

func TestDeleteProjectRemovesAllSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid, otherPID := store.MustNewUUID(), store.MustNewUUID()
	docA, docB, otherDoc := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	_, err := s.IndexSource(ctx, pid, "document", docA, "a.pdf", "对比学习 self-supervised 表示学习范式。")
	require.NoError(t, err)
	_, err = s.IndexSource(ctx, pid, "wiki", docB, "笔记", "对比学习的负样本构造。")
	require.NoError(t, err)
	_, err = s.IndexSource(ctx, otherPID, "document", otherDoc, "other.pdf", "对比学习在推荐系统的应用。")
	require.NoError(t, err)

	require.NoError(t, s.DeleteProject(ctx, pid))

	hits, err := s.Search(ctx, pid, "对比学习", 3)
	require.NoError(t, err)
	assert.Empty(t, hits, "项目删除后不得再命中其任何来源")

	other, err := s.Search(ctx, otherPID, "对比学习", 3)
	require.NoError(t, err)
	assert.NotEmpty(t, other, "其他项目的索引不受影响")
}
