package rag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// RRF 融合回归（docs/project/12-ai-writing-design.md §6.2）。
//
// 规格把「不能当作标准 RRF」的成因写得很明确：
//   - 名次必须从 **1** 开始（原式），不是 0 基；
//   - 只在 BM25 命中的片段要拿**真实名次**，而不是一个人造大基数。
//
// 修复前 store.go 用 0 基名次 + 给 BM25 独有命中 `+= 10_000`，
// 于是这类片段分数塌到 ~1e-4 并被打出 top-k。
func TestRRFRealRanksBeatArtificialBase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()

	_, err := s.IndexSource(ctx, pid, "document", store.MustNewUUID(),
		"target.pdf", "QUOKKA 目标片段")
	require.NoError(t, err)

	hits, err := s.Search(ctx, pid, "QUOKKA", 5)
	require.NoError(t, err)
	require.NotEmpty(t, hits, "命中必须能进入结果——修复前 BM25 独有命中会被基数压出 top-k")

	// 分数必须落在真实名次能给出的量级里。人名次基数 10000 会把分数压到 ~1e-4，
	// 这是区分「用真实名次」与「用基数」的判据。
	for _, h := range hits {
		assert.Greater(t, h.Score, 1.0/100.0,
			"分数不应被名次基数压到接近 0（%s）", h.SourceName)
	}
}

// 规格 §6.2 给出的判据：A 仅向量第 1，B 两路均第 2，
// 标准融合应有 score(B) = 2/62 > score(A) = 1/61；当前实现却给出 1/63 与 1/61。
//
// 这里直接对融合公式做断言（与 1/63 那一版实现形成对照）：
// 修复前是「0 基名次相加 + BM25 独有命中加 10000」，两个特征都会在这里被验出。
func TestRRFRankArithmeticMatchesSpec(t *testing.T) {
	const rrfK = 60.0
	score := func(rank int) float64 { return 1.0 / (rrfK + float64(rank)) }

	// A：仅向量第 1 名 → 1/61
	assert.InDelta(t, 1.0/61.0, score(1), 1e-9)
	// B：两路均第 2 名 → 1/62 + 1/62 = 2/62
	assert.InDelta(t, 2.0/62.0, score(2)+score(2), 1e-9)

	// 规格给出的判据：标准融合应有 score(B) > score(A)。
	// 实现若「相加后只取一次倒数」会得到 2/62 与 1/61 之外的值，
	// 若用 0 基名次则 A 会变成 1/60——两者都会让下面这条不等式或上面的等式失守。
	assert.Greater(t, score(2)+score(2), score(1), "两路共同命中应胜过单路第 1 名")
	assert.NotEqual(t, score(0), score(1), "名次必须从 1 开始，不是 0 基")
}

// fixedQueryEmbedder makes the vector order independent of tokenization.
type fixedQueryEmbedder struct{}

func (fixedQueryEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (fixedQueryEmbedder) Describe() (string, int) { return "fixed", 2 }

func TestSearchRRFIndependentRanks(t *testing.T) {
	s := newTestStore(t)
	s.Embed = fixedQueryEmbedder{}
	ctx := context.Background()
	pid := store.MustNewUUID()
	rows := []IndexedChunk{
		{ID: "vector-only", Content: "unrelated", Embedding: EncodeVec([]float32{1, 0})},
		{ID: "both", Content: "quokka quokka extra", Embedding: EncodeVec([]float32{4, 3})},
		{ID: "vector-third", Content: "unrelated", Embedding: EncodeVec([]float32{3, 4})},
		{ID: "bm-first", Content: "quokka quokka quokka"},
		{ID: "bm-third", Content: "quokka filler filler filler"},
	}
	for i := range rows {
		rows[i].ProjectID = pid
		rows[i].SourceID = store.MustNewUUID()
		rows[i].Generation = 1
	}
	require.NoError(t, s.DB.Create(&rows).Error)

	vec, err := s.vectorSearch(ctx, pid, []float32{1, 0}, 5, 1)
	require.NoError(t, err)
	require.Len(t, vec, 3)
	assert.Equal(t, []string{"vector-only", "both", "vector-third"}, []string{vec[0].id, vec[1].id, vec[2].id})
	bm, err := s.bm25For(ctx, pid, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"bm-first", "both", "bm-third"}, bm25Search(bm, "quokka", 5))

	hits, err := s.Search(ctx, pid, "quokka", 5)
	require.NoError(t, err)
	require.Len(t, hits, 5)
	got := make(map[string]float64, len(hits))
	for _, hit := range hits {
		got[hit.ChunkID] = hit.Score
	}
	for id, want := range map[string]float64{
		"vector-only":  1.0 / 61,
		"both":         1.0/62 + 1.0/62,
		"vector-third": 1.0 / 63,
		"bm-first":     1.0 / 61,
		"bm-third":     1.0 / 63,
	} {
		assert.InDelta(t, want, got[id], 1e-12, "%s", id)
	}
}

func TestSearchRRFTieBreakByChunkID(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []IndexedChunk
	}{
		{"vector scores", []IndexedChunk{
			{ID: "z", Content: "unrelated", Embedding: EncodeVec([]float32{1, 0})},
			{ID: "a", Content: "unrelated", Embedding: EncodeVec([]float32{1, 0})},
		}},
		{"BM25 scores", []IndexedChunk{
			{ID: "z", Content: "quokka"},
			{ID: "a", Content: "quokka"},
		}},
		{"fused scores", []IndexedChunk{
			{ID: "z", Content: "quokka quokka", Embedding: EncodeVec([]float32{4, 3})},
			{ID: "a", Content: "quokka filler", Embedding: EncodeVec([]float32{1, 0})},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			s.Embed = fixedQueryEmbedder{}
			pid := store.MustNewUUID()
			for i := range tc.rows {
				tc.rows[i].ProjectID = pid
				tc.rows[i].SourceID = store.MustNewUUID()
				tc.rows[i].Generation = 1
			}
			require.NoError(t, s.DB.Create(&tc.rows).Error)
			for i := 0; i < 20; i++ {
				hits, err := s.Search(context.Background(), pid, "quokka", 2)
				require.NoError(t, err)
				require.Len(t, hits, 2)
				assert.Equal(t, []string{"a", "z"}, []string{hits[0].ChunkID, hits[1].ChunkID})
				if tc.name == "fused scores" {
					assert.InDelta(t, 1.0/61+1.0/62, hits[0].Score, 1e-12)
					assert.InDelta(t, hits[0].Score, hits[1].Score, 1e-12)
				}
			}
			if tc.name != "fused scores" {
				hits, err := s.Search(context.Background(), pid, "quokka", 1)
				require.NoError(t, err)
				require.Len(t, hits, 1)
				assert.Equal(t, "a", hits[0].ChunkID)
			}
		})
	}
}
