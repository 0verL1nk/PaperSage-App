package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Store — hybrid 检索门面：向量（内存余弦，DB 持久）+ BM25（内存倒排）+ RRF。
//
// 真相源唯一：rag_chunks 表（含 embedding JSON 列）。向量与 BM25 的
// 进程内索引均从该表惰性构建（每项目一次），重启后只 reload、绝不
// 重新嵌入——嵌入调用只发生在 IndexSource（写路径）与查询文本（读路径）。
type Store struct {
	DB    *gorm.DB
	Embed Embedder

	mu   sync.RWMutex
	vec  map[string]*vecIndex
	bm25 map[string]*bm25Index
}

// vecIndex 项目级向量索引（内存）。
type vecIndex struct {
	entries []vecEntry
}

type vecEntry struct {
	id      string
	vec     []float32
	norm    float64 // 预归一化范数
	content string
	stype   string // source_type
	sid     string // source_id
	sname   string
}

// NewStore（持久目录参数保留兼容签名；向量真相源在 DB，无需额外目录）。
func NewStore(db *gorm.DB, emb Embedder, _ string) (*Store, error) {
	return &Store{
		DB: db, Embed: emb,
		vec: map[string]*vecIndex{}, bm25: map[string]*bm25Index{},
	}, nil
}

// ── 索引 ─────────────────────────────────────────────

// IndexedChunk 落库的真相源行（含向量）。
type IndexedChunk struct {
	ID         string     `gorm:"column:id;primaryKey;size:80"`
	ProjectID  store.UUID `gorm:"column:project_id;index"`
	SourceType string     `gorm:"column:source_type;size:20"`
	SourceID   store.UUID `gorm:"column:source_id;index"`
	SourceName string     `gorm:"column:source_name;size:255"`
	ChunkIdx   int        `gorm:"column:chunk_idx"`
	Content    string     `gorm:"column:content"`
	Embedding  string     `gorm:"column:embedding"` // JSON []float32
}

func (IndexedChunk) TableName() string { return "rag_chunks" }

// IndexSource 对一个来源重建索引：
// 删旧行 → 分块 → 嵌入（唯一嵌入点）→ 整行（含向量）落 DB → 失效内存索引。
func (s *Store) IndexSource(ctx context.Context, projectID store.UUID,
	sourceType string, sourceID store.UUID, sourceName, text string) (int, error) {

	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND source_type = ? AND source_id = ?",
			projectID, sourceType, sourceID).
		Delete(&IndexedChunk{}).Error; err != nil {
		return 0, err
	}

	chunks := SplitChunks(text, 1200, 150)
	if len(chunks) == 0 {
		s.invalidate(projectID)
		return 0, nil
	}

	// 嵌入（批量；写路径唯一嵌入点）
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	vecs, err := s.Embed.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}

	rows := make([]IndexedChunk, len(chunks))
	for i, c := range chunks {
		raw, err := json.Marshal(vecs[i])
		if err != nil {
			return 0, err
		}
		rows[i] = IndexedChunk{
			ID:        fmt.Sprintf("%s:%s:%d", sourceType, sourceID.String(), c.Idx),
			ProjectID: projectID, SourceType: sourceType,
			SourceID: sourceID, SourceName: sourceName,
			ChunkIdx: c.Idx, Content: c.Content,
			Embedding: string(raw),
		}
	}
	if err := s.DB.WithContext(ctx).Create(&rows).Error; err != nil {
		return 0, err
	}
	s.invalidate(projectID)
	return len(chunks), nil
}

func (s *Store) DeleteSource(ctx context.Context, projectID store.UUID,
	sourceType string, sourceID store.UUID) error {
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND source_type = ? AND source_id = ?",
			projectID, sourceType, sourceID).
		Delete(&IndexedChunk{}).Error; err != nil {
		return err
	}
	s.invalidate(projectID)
	return nil
}

// ── 检索（hybrid + RRF）──────────────────────────────

// Hit 单条检索结果。
type Hit struct {
	ChunkID    string  `json:"chunkId"`
	SourceType string  `json:"sourceType"`
	SourceID   string  `json:"sourceId"`
	SourceName string  `json:"sourceName"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

const rrfK = 60.0

// Search 混合检索：向量 top-k 与 BM25 top-k，RRF 融合。
// 查询文本嵌入是本函数唯一的嵌入调用。
func (s *Store) Search(ctx context.Context, projectID store.UUID, query string, k int) ([]Hit, error) {
	if k <= 0 {
		k = 5
	}
	ranks := map[string]int{}
	hits := map[string]Hit{}

	// 向量路
	qvecs, err := s.Embed.Embed(ctx, []string{query})
	if err == nil && len(qvecs) == 1 {
		for rank, e := range s.vectorSearch(ctx, projectID, qvecs[0], k) {
			ranks[e.id] += rank
			hits[e.id] = Hit{
				ChunkID: e.id, Content: e.content,
				SourceType: e.stype, SourceID: e.sid, SourceName: e.sname,
			}
		}
	}

	// BM25 路
	for rank, id := range s.bm25Search(projectID, query, k) {
		if _, ok := hits[id]; ok {
			ranks[id] += rank
			continue
		}
		var row IndexedChunk
		if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
			continue
		}
		hits[id] = Hit{
			ChunkID: row.ID, Content: row.Content,
			SourceType: row.SourceType, SourceID: row.SourceID.String(),
			SourceName: row.SourceName,
		}
		ranks[id] += 10_000 // 未进向量路的 BM25 命中给大名次基数
	}

	out := make([]Hit, 0, len(hits))
	for id, h := range hits {
		h.Score = 1.0 / (rrfK + float64(ranks[id]) + 1)
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func (s *Store) invalidate(projectID store.UUID) {
	key := projectID.String()
	s.mu.Lock()
	delete(s.vec, key)
	delete(s.bm25, key)
	s.mu.Unlock()
}

// ── 向量索引（内存，从 DB 惰性加载——只 reload 不重嵌入）────

func (s *Store) vectorIndex(ctx context.Context, projectID store.UUID) *vecIndex {
	key := projectID.String()
	s.mu.RLock()
	if idx, ok := s.vec[key]; ok {
		s.mu.RUnlock()
		return idx
	}
	s.mu.RUnlock()

	var rows []IndexedChunk
	_ = s.DB.WithContext(ctx).
		Select("id", "content", "source_type", "source_id", "source_name", "embedding").
		Where("project_id = ?", projectID).Find(&rows).Error

	idx := &vecIndex{}
	for _, r := range rows {
		if r.Embedding == "" {
			continue
		}
		var v []float32
		if json.Unmarshal([]byte(r.Embedding), &v) != nil || len(v) == 0 {
			continue
		}
		n := 0.0
		for _, x := range v {
			n += float64(x) * float64(x)
		}
		n = math.Sqrt(n)
		if n == 0 {
			continue
		}
		idx.entries = append(idx.entries, vecEntry{
			id: r.ID, vec: v, norm: n, content: r.Content,
			stype: r.SourceType, sid: r.SourceID.String(), sname: r.SourceName,
		})
	}
	s.mu.Lock()
	s.vec[key] = idx
	s.mu.Unlock()
	return idx
}

// vectorSearch 余弦 top-k（与 chromem 同为暴力精确检索，数据量级内够用）。
func (s *Store) vectorSearch(ctx context.Context, projectID store.UUID, q []float32, k int) []vecEntry {
	idx := s.vectorIndex(ctx, projectID)
	if len(idx.entries) == 0 {
		return nil
	}
	qn := 0.0
	for _, x := range q {
		qn += float64(x) * float64(x)
	}
	qn = math.Sqrt(qn)
	if qn == 0 {
		return nil
	}
	type scored struct {
		e vecEntry
		s float64
	}
	list := make([]scored, 0, len(idx.entries))
	for _, e := range idx.entries {
		dot := 0.0
		for i, x := range e.vec {
			if i < len(q) {
				dot += float64(x) * float64(q[i])
			}
		}
		if cos := dot / (e.norm * qn); cos > 1e-6 {
			// 阈值截断：零重叠（正交）不进候选——否则小库内
			// top-k 会把 0 分条目也带出来
			list = append(list, scored{e, cos})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].s > list[j].s })
	if len(list) > k {
		list = list[:k]
	}
	out := make([]vecEntry, len(list))
	for i, x := range list {
		out[i] = x.e
	}
	return out
}

// ── BM25（纯 Go 内存倒排，项目级惰性构建）────────────

type bm25Index struct {
	docs []string // chunkID 列表
	tf   map[string]map[string]int
	df   map[string]int
	avg  float64
}

func (s *Store) bm25For(projectID store.UUID) *bm25Index {
	key := projectID.String()
	s.mu.RLock()
	if idx, ok := s.bm25[key]; ok {
		s.mu.RUnlock()
		return idx
	}
	s.mu.RUnlock()

	var rows []IndexedChunk
	_ = s.DB.Where("project_id = ?", projectID).Find(&rows).Error
	idx := &bm25Index{
		tf: map[string]map[string]int{},
		df: map[string]int{},
	}
	total := 0
	for _, r := range rows {
		idx.docs = append(idx.docs, r.ID)
		toks := tokenize(r.Content)
		tf := map[string]int{}
		seen := map[string]bool{}
		for _, t := range toks {
			tf[t]++
			if !seen[t] {
				seen[t] = true
				idx.df[t]++
			}
		}
		idx.tf[r.ID] = tf
		total += len(toks)
	}
	if len(rows) > 0 {
		idx.avg = float64(total) / float64(len(rows))
	}
	s.mu.Lock()
	s.bm25[key] = idx
	s.mu.Unlock()
	return idx
}

func (s *Store) bm25Search(projectID store.UUID, query string, k int) []string {
	idx := s.bm25For(projectID)
	if len(idx.docs) == 0 {
		return nil
	}
	N := float64(len(idx.docs))
	k1, b := 1.2, 0.75
	type scored struct {
		id string
		s  float64
	}
	var list []scored
	for _, id := range idx.docs {
		tf := idx.tf[id]
		dl := 0.0
		for _, v := range tf {
			dl += float64(v)
		}
		var sc float64
		for _, q := range tokenize(query) {
			f := float64(tf[q])
			if f == 0 {
				continue
			}
			n := float64(idx.df[q])
			idf := math.Log(1 + (N-n+0.5)/(n+0.5))
			sc += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*dl/max(idx.avg, 1)))
		}
		if sc > 0 {
			list = append(list, scored{id, sc})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].s > list[j].s })
	if len(list) > k {
		list = list[:k]
	}
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.id
	}
	return out
}

var _ = strings.TrimSpace
