package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/philippgille/chromem-go"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Store — hybrid 检索门面：向量（chromem）+ BM25（内存倒排）+ RRF。
type Store struct {
	DB      *gorm.DB
	Embed   Embedder
	chromem *chromem.DB

	mu   sync.RWMutex
	bm25 map[string]*bm25Index // projectID(hex) → 索引（惰性构建）
}

func NewStore(db *gorm.DB, emb Embedder, persistDir string) (*Store, error) {
	var cdb *chromem.DB
	var err error
	if persistDir != "" {
		cdb, err = chromem.NewPersistentDB(persistDir, false)
	} else {
		cdb = chromem.NewDB()
	}
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, Embed: emb, chromem: cdb, bm25: map[string]*bm25Index{}}, nil
}

func (s *Store) collection(projectID store.UUID) (*chromem.Collection, error) {
	embFn := func(ctx context.Context, text string) ([]float32, error) {
		v, err := s.Embed.Embed(ctx, []string{text})
		if err != nil || len(v) == 0 {
			return nil, err
		}
		return v[0], nil
	}
	return s.chromem.GetOrCreateCollection("p-"+projectID.String(), nil, embFn)
}

// ── 索引 ─────────────────────────────────────────────

// IndexedChunk 落库的真相源行。
type IndexedChunk struct {
	ID         string     `gorm:"column:id;primaryKey;size:80"`
	ProjectID  store.UUID `gorm:"column:project_id;index"`
	SourceType string     `gorm:"column:source_type;size:20"`
	SourceID   store.UUID `gorm:"column:source_id;index"`
	SourceName string     `gorm:"column:source_name;size:255"`
	ChunkIdx   int        `gorm:"column:chunk_idx"`
	Content    string     `gorm:"column:content"`
}

func (IndexedChunk) TableName() string { return "rag_chunks" }

// IndexSource 对一个来源（document 或 wiki 页）重建索引：
// 删旧块 → 抽块 → 嵌入 → 双写（DB 真相源 + chromem + BM25 失效）。
func (s *Store) IndexSource(ctx context.Context, projectID store.UUID,
	sourceType string, sourceID store.UUID, sourceName, text string) (int, error) {

	// 1) 清旧
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND source_type = ? AND source_id = ?",
			projectID, sourceType, sourceID).
		Delete(&IndexedChunk{}).Error; err != nil {
		return 0, err
	}
	col, err := s.collection(projectID)
	if err != nil {
		return 0, err
	}
	_ = col.Delete(ctx, nil, nil, idsForSource(sourceType, sourceID)...)

	chunks := SplitChunks(text, 1200, 150)
	if len(chunks) == 0 {
		s.invalidateBM25(projectID)
		return 0, nil
	}

	// 2) 嵌入（批量）
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	vecs, err := s.Embed.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}

	// 3) 双写
	ids := make([]string, len(chunks))
	metas := make([]map[string]string, len(chunks))
	rows := make([]IndexedChunk, len(chunks))
	for i, c := range chunks {
		ids[i] = fmt.Sprintf("%s:%s:%d", sourceType, sourceID.String(), c.Idx)
		metas[i] = map[string]string{
			"project_id":  projectID.String(),
			"source_type": sourceType,
			"source_id":   sourceID.String(),
			"source_name": sourceName,
		}
		rows[i] = IndexedChunk{
			ID: ids[i], ProjectID: projectID, SourceType: sourceType,
			SourceID: sourceID, SourceName: sourceName,
			ChunkIdx: c.Idx, Content: c.Content,
		}
	}
	if err := s.DB.WithContext(ctx).Create(&rows).Error; err != nil {
		return 0, err
	}
	if err := col.Add(ctx, ids, vecs, metas, texts); err != nil {
		return 0, fmt.Errorf("vector add: %w", err)
	}
	s.invalidateBM25(projectID)
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
	if col, err := s.collection(projectID); err == nil {
		_ = col.Delete(ctx, nil, nil, idsForSource(sourceType, sourceID)...)
	}
	s.invalidateBM25(projectID)
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
func (s *Store) Search(ctx context.Context, projectID store.UUID, query string, k int) ([]Hit, error) {
	if k <= 0 {
		k = 5
	}
	ranks := map[string]int{} // chunkID → 两路名次
	hits := map[string]Hit{}

	if col, err := s.collection(projectID); err == nil {
		res, err := col.Query(ctx, query, k, nil, nil)
		if err == nil {
			for rank, r := range res {
				id := r.ID
				ranks[id] += rank
				hits[id] = Hit{
					ChunkID: id, Content: r.Content,
					SourceType: r.Metadata["source_type"],
					SourceID:   r.Metadata["source_id"],
					SourceName: r.Metadata["source_name"],
				}
			}
		}
	}

	for rank, id := range s.bm25Search(projectID, query, k) {
		if _, ok := hits[id]; !ok {
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
		} else {
			ranks[id] += rank
		}
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

func (s *Store) invalidateBM25(projectID store.UUID) {
	s.mu.Lock()
	delete(s.bm25, projectID.String())
	s.mu.Unlock()
}

// ── BM25（纯 Go 内存倒排，项目级惰性构建）────────────

type bm25Index struct {
	docs    []string          // chunkID 列表
	content map[string]string // chunkID → 内容（复用 tokenize）
	tf      map[string]map[string]int
	df      map[string]int
	avgLen  float64
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
		content: map[string]string{},
		tf:      map[string]map[string]int{},
		df:      map[string]int{},
	}
	total := 0
	for _, r := range rows {
		idx.docs = append(idx.docs, r.ID)
		toks := tokenize(r.Content)
		idx.content[r.ID] = r.Content
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
		idx.avgLen = float64(total) / float64(len(rows))
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
		dl := float64(0)
		for _, v := range tf {
			dl += float64(v)
		}
		var score float64
		for _, q := range tokenize(query) {
			f := float64(tf[q])
			if f == 0 {
				continue
			}
			n := float64(idx.df[q])
			idf := math.Log(1 + (N-n+0.5)/(n+0.5))
			score += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*dl/max(idx.avgLen, 1)))
		}
		if score > 0 {
			list = append(list, scored{id, score})
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

// idsForSource：按 chunk ID 规约删除（chromem Delete 按 ID 最稳）。
func idsForSource(sourceType string, sourceID store.UUID) []string {
	// 保守上限 512 块/来源（SplitChunks 目标 1200 字符，覆盖 ~60 万字）
	out := make([]string, 0, 64)
	for i := 0; i < 512; i++ {
		out = append(out, fmt.Sprintf("%s:%s:%d", sourceType, sourceID.String(), i))
	}
	return out
}

var _ = strings.TrimSpace
