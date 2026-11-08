package rag

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Store — hybrid 检索门面：向量（内存余弦，DB 持久）+ BM25（内存倒排）+ RRF。
//
// 真相源唯一：rag_chunks 表（含 embedding JSON 列）。向量与 BM25 的
// 进程内索引均从该表惰性构建（每项目一次），重启后只 reload、绝不
// 重新嵌入——嵌入调用只发生在 IndexSource（写路径）与查询文本（读路径）。
type Store struct {
	DB         *gorm.DB
	Embed      Embedder
	Pg         *PgStore // Neon(pgvector) 后端；配置了 NEON_DATABASE_URL 时启用
	vectorInDB bool     // TiDB：VECTOR 列库端检索；普通 MySQL 使用二进制向量 + 内存检索

	mu      sync.RWMutex
	vec     map[string]*vecIndex
	bm25    map[string]*bm25Index
	version map[string]uint64 // invalidation epochs prevent stale in-flight loads from repopulating caches
}

// vecIndex 项目级向量索引（内存）。
type vecIndex struct {
	generation int64
	entries    []vecEntry
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
	inDB := false
	if db != nil { // nil DB：纯 Neon 后端模式（集成测试）
		if db.Dialector.Name() == "mysql" { //nolint:staticcheck // QF1008: 方法而非嵌入字段
			var version string
			if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
				return nil, fmt.Errorf("detect MySQL vector capabilities: %w", err)
			}
			inDB = strings.Contains(strings.ToLower(version), "tidb")
		}
	}
	return &Store{
		DB: db, Embed: emb, vectorInDB: inDB,
		vec: map[string]*vecIndex{}, bm25: map[string]*bm25Index{}, version: map[string]uint64{},
	}, nil
}

// AttachNeon 挂载 Neon(pgvector) 后端（配置了 NEON_DATABASE_URL 时调用）。
// 挂载后 chunk 写入/向量检索/BM25 内容源全部走 PG；主库不再存 chunk。
func (s *Store) AttachNeon(ctx context.Context, dsn string) error {
	pg, err := NewPgStore(ctx, dsn)
	if err != nil {
		return err
	}
	s.Pg = pg
	return nil
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
	Embedding  []byte     `gorm:"column:embedding"` // binary float32 小端
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"`
	// Generation 标识片段属于哪一代索引；检索只命中项目当前 active 的代次。
	Generation int64 `gorm:"column:generation;index"`
}

func (IndexedChunk) TableName() string { return "rag_chunks" }

// IndexSource 对一个来源重建索引：
// 分块 → 嵌入（唯一嵌入点）→ 事务内替换旧行 → 失效内存索引。
// 先嵌入再删除：嵌入失败时旧索引保持可用。
func (s *Store) IndexSource(ctx context.Context, projectID store.UUID,
	sourceType string, sourceID store.UUID, sourceName, text string) (int, error) {

	if s.Pg == nil && s.DB == nil {
		return 0, fmt.Errorf("no store backend")
	}

	chunks := SplitChunks(text, 1200, 150)
	if len(chunks) == 0 {
		// 空内容也必须清掉该来源旧行，否则旧索引会继续被检索到。
		if s.Pg != nil {
			if err := s.Pg.DeleteSource(ctx, hexID(projectID), sourceType, hexID(sourceID)); err != nil {
				return 0, err
			}
		} else if err := s.DB.WithContext(ctx).
			Where("project_id = ? AND source_type = ? AND source_id = ?",
				projectID, sourceType, sourceID).
			Delete(&IndexedChunk{}).Error; err != nil {
			return 0, err
		}
		s.invalidate(projectID)
		return 0, nil
	}

	// 片段归属写侧代次（构建中的优先，否则活动代次）；检索只命中活动代次。
	gen, err := s.WriteGeneration(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("resolve write generation: %w", err)
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
		rows[i] = IndexedChunk{
			ID:        fmt.Sprintf("%s:%s:%d", sourceType, sourceID.String(), c.Idx),
			ProjectID: projectID, SourceType: sourceType,
			SourceID: sourceID, SourceName: sourceName,
			ChunkIdx: c.Idx, Content: c.Content,
			Embedding:  EncodeVec(vecs[i]),
			Generation: gen,
		}
	}
	if s.Pg != nil {
		// Neon：chunk 与向量整体落 PG（主库不留副本，单一真相源）。
		// hex 无横线（32 位）与主库列形态一致
		if err := s.Pg.IndexSource(ctx, hexID(projectID), sourceType,
			hexID(sourceID), sourceName, chunks, vecs, gen); err != nil {
			return 0, err
		}
	} else if s.vectorInDB {
		// TiDB VECTOR 列以 '[f1,f2,…]' 字面量写入
		err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("project_id = ? AND source_type = ? AND source_id = ?",
				projectID, sourceType, sourceID).Delete(&IndexedChunk{}).Error; err != nil {
				return err
			}
			return s.insertVectorRows(ctx, tx, rows, vecs)
		})
	} else {
		err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("project_id = ? AND source_type = ? AND source_id = ?",
				projectID, sourceType, sourceID).Delete(&IndexedChunk{}).Error; err != nil {
				return err
			}
			return tx.Create(&rows).Error
		})
	}
	if err != nil {
		return 0, err
	}
	s.invalidate(projectID)
	return len(chunks), nil
}

func (s *Store) DeleteSource(ctx context.Context, projectID store.UUID,
	sourceType string, sourceID store.UUID) error {
	if s.Pg != nil {
		if err := s.Pg.DeleteSource(ctx, hexID(projectID), sourceType, hexID(sourceID)); err != nil {
			return err
		}
		s.invalidate(projectID)
		return nil
	}
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND source_type = ? AND source_id = ?",
			projectID, sourceType, sourceID).
		Delete(&IndexedChunk{}).Error; err != nil {
		return err
	}
	s.invalidate(projectID)
	return nil
}

// DeleteProject 删一个项目的全部块（永久删除项目时调用）。
func (s *Store) DeleteProject(ctx context.Context, projectID store.UUID) error {
	if s.Pg != nil {
		if err := s.Pg.DeleteProject(ctx, hexID(projectID)); err != nil {
			return err
		}
		s.invalidate(projectID)
		return nil
	}
	if err := s.DB.WithContext(ctx).
		Where("project_id = ?", projectID).
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

// Search 混合检索：向量 top-k 与 BM25 top-k，RRF 融合。
// 查询文本嵌入是本函数唯一的嵌入调用。
func (s *Store) Search(ctx context.Context, projectID store.UUID, query string, k int) ([]Hit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.DB == nil && s.Pg == nil {
		return nil, fmt.Errorf("no store backend")
	}
	if k <= 0 {
		k = 5
	}
	// 检索只命中活动代次；下面的各条检索路径都以此为准。
	gen, err := s.ActiveGeneration(ctx, projectID)
	if err != nil {
		return nil, err
	}
	// RRF：score(d) = Σ_i 1/(k + rank_i(d))，名次从 1 开始，逐路取倒数后**相加**。
	// 未进入某路候选列表时该路贡献为 0；同一路重复片段只记一次。
	// （docs/project/12-ai-writing-design.md §6.2）
	const rrfK = 60.0
	scores := map[string]float64{}
	hits := map[string]Hit{}
	addRanked := func(id string, rank int) {
		scores[id] += 1.0 / (rrfK + float64(rank))
	}

	// 向量路
	vectorSeen := map[string]bool{}
	qvecs, err := s.Embed.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(qvecs) != 1 || len(qvecs[0]) == 0 {
		return nil, fmt.Errorf("embed query: expected one nonempty vector")
	}
	if s.Pg != nil {
		pgHits, err := s.Pg.SearchVector(ctx, hexID(projectID), qvecs[0], k, gen)
		if err != nil {
			return nil, fmt.Errorf("search vectors: %w", err)
		}
		for i, r := range pgHits {
			if vectorSeen[r.ID] {
				continue
			}
			vectorSeen[r.ID] = true
			addRanked(r.ID, i+1)
			hits[r.ID] = Hit{
				ChunkID: r.ID, Content: r.Content,
				SourceType: r.SourceType, SourceID: r.SourceID,
				SourceName: r.SourceName, Score: r.Score,
			}
		}
	} else {
		var vecHits []vecEntry
		if s.vectorInDB {
			vecHits, err = s.vectorSearchDB(ctx, projectID, qvecs[0], k, gen)
		} else {
			vecHits, err = s.vectorSearch(ctx, projectID, qvecs[0], k, gen)
		}
		if err != nil {
			return nil, fmt.Errorf("search vectors: %w", err)
		}
		for i, e := range vecHits {
			if vectorSeen[e.id] {
				continue
			}
			vectorSeen[e.id] = true
			addRanked(e.id, i+1)
			hits[e.id] = Hit{
				ChunkID: e.id, Content: e.content,
				SourceType: e.stype, SourceID: e.sid, SourceName: e.sname,
			}
		}
	}

	// BM25 路
	bm, err := s.bm25For(ctx, projectID, gen)
	if err != nil {
		return nil, fmt.Errorf("load BM25 index: %w", err)
	}
	bmIDs := bm25Search(bm, query, k)
	var missing []string
	for _, id := range bmIDs {
		if _, ok := hits[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		if s.Pg != nil {
			rows, err := s.Pg.RowsByID(ctx, hexID(projectID), gen, missing)
			if err != nil {
				return nil, fmt.Errorf("load BM25 candidates: %w", err)
			}
			for _, row := range rows {
				hits[row.ID] = Hit{ChunkID: row.ID, Content: row.Content, SourceType: row.SourceType, SourceID: row.SourceID, SourceName: row.SourceName}
			}
		} else {
			var rows []IndexedChunk
			if err := s.DB.WithContext(ctx).Select("id", "content", "source_type", "source_id", "source_name").Where("project_id = ? AND generation = ? AND id IN ?", projectID, gen, missing).Find(&rows).Error; err != nil {
				return nil, fmt.Errorf("load BM25 candidates: %w", err)
			}
			for _, row := range rows {
				hits[row.ID] = Hit{ChunkID: row.ID, Content: row.Content, SourceType: row.SourceType, SourceID: row.SourceID.String(), SourceName: row.SourceName}
			}
		}
		for _, id := range missing {
			if _, ok := hits[id]; !ok {
				return nil, fmt.Errorf("BM25 candidate %q no longer exists in project generation", id)
			}
		}
	}
	bmSeen := map[string]bool{}
	for i, id := range bmIDs {
		if !bmSeen[id] {
			bmSeen[id] = true
			addRanked(id, i+1)
		}
	}

	out := make([]Hit, 0, len(hits))
	for id, h := range hits {
		h.Score = scores[id]
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ChunkID < out[j].ChunkID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) invalidate(projectID store.UUID) {
	key := projectID.String()
	s.mu.Lock()
	s.version[key]++
	delete(s.vec, key)
	delete(s.bm25, key)
	s.mu.Unlock()
}

// ── 向量索引（内存，从 DB 惰性加载——只 reload 不重嵌入）────

func (s *Store) vectorIndex(ctx context.Context, projectID store.UUID, gen int64) (*vecIndex, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := projectID.String()
	s.mu.RLock()
	if idx, ok := s.vec[key]; ok && idx.generation == gen {
		s.mu.RUnlock()
		return idx, nil
	}
	version := s.version[key]
	s.mu.RUnlock()

	var rows []IndexedChunk
	if err := s.DB.WithContext(ctx).
		Select("id", "content", "source_type", "source_id", "source_name", "embedding").
		Where("project_id = ? AND generation = ?", projectID, gen).Find(&rows).Error; err != nil {
		return nil, err
	}

	idx := &vecIndex{generation: gen}
	for _, r := range rows {
		if len(r.Embedding) == 0 {
			continue
		}
		v := DecodeVec(r.Embedding)
		if len(v) == 0 {
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
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.version[key] != version {
		return nil, fmt.Errorf("vector index invalidated while loading")
	}
	s.vec[key] = idx
	return idx, nil
}

// vectorSearch 余弦 top-k（与 chromem 同为暴力精确检索，数据量级内够用）。
func (s *Store) vectorSearch(ctx context.Context, projectID store.UUID, q []float32, k int, gen int64) ([]vecEntry, error) {
	idx, err := s.vectorIndex(ctx, projectID, gen)
	if err != nil {
		return nil, err
	}
	if len(idx.entries) == 0 {
		return nil, nil
	}
	qn := 0.0
	for _, x := range q {
		qn += float64(x) * float64(x)
	}
	qn = math.Sqrt(qn)
	if qn == 0 {
		return nil, nil
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
	sort.Slice(list, func(i, j int) bool {
		if list[i].s == list[j].s {
			return list[i].e.id < list[j].e.id
		}
		return list[i].s > list[j].s
	})
	if len(list) > k {
		list = list[:k]
	}
	out := make([]vecEntry, len(list))
	for i, x := range list {
		out[i] = x.e
	}
	return out, nil
}

// ── BM25（纯 Go 内存倒排，项目级惰性构建）────────────

type bm25Index struct {
	docs []string // chunkID 列表
	tf   map[string]map[string]int
	df   map[string]int
	avg  float64
	// generation 记录本索引由哪一代片段构建，供检索路径校验，避免混入其他代次。
	generation int64
}

func (s *Store) bm25For(ctx context.Context, projectID store.UUID, gen int64) (*bm25Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := projectID.String()
	s.mu.RLock()
	if idx, ok := s.bm25[key]; ok && idx.generation == gen {
		s.mu.RUnlock()
		return idx, nil
	}
	version := s.version[key]
	s.mu.RUnlock()

	var rows []IndexedChunk
	if s.Pg != nil {
		pgRows, err := s.Pg.SearchBM25(ctx, hexID(projectID), gen)
		if err != nil {
			return nil, err
		}
		for _, r := range pgRows {
			rows = append(rows, IndexedChunk{ID: r.ID, Content: r.Content})
		}
	} else if err := s.DB.WithContext(ctx).Select("id", "content").Where("project_id = ? AND generation = ?", projectID, gen).Find(&rows).Error; err != nil {
		return nil, err
	}
	idx := &bm25Index{
		tf:         map[string]map[string]int{},
		df:         map[string]int{},
		generation: gen,
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
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.version[key] != version {
		return nil, fmt.Errorf("BM25 index invalidated while loading")
	}
	s.bm25[key] = idx
	return idx, nil
}

func bm25Search(idx *bm25Index, query string, k int) []string {
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
	sort.Slice(list, func(i, j int) bool {
		if list[i].s == list[j].s {
			return list[i].id < list[j].id
		}
		return list[i].s > list[j].s
	})
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

// EncodeVec []float32 → 小端 binary（每维 4 字节，定长）。
func EncodeVec(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

// DecodeVec binary → []float32（长度非 4 倍数返回 nil）。
func DecodeVec(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// insertVectorRows TiDB 方言：embedding 列以向量字面量插入
// （go-sql-driver 对 VECTOR 列按字符串交互）。
func (s *Store) insertVectorRows(ctx context.Context, tx *gorm.DB, rows []IndexedChunk, vecs [][]float32) error {
	for i, r := range rows {
		if err := tx.WithContext(ctx).Exec(
			`INSERT INTO rag_chunks (id, project_id, source_type, source_id, source_name, chunk_idx, content, embedding, generation, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(6))`,
			r.ID, r.ProjectID, r.SourceType, r.SourceID, r.SourceName,
			r.ChunkIdx, r.Content, VecLiteral(vecs[i]), r.Generation,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// vectorSearchDB 库端余弦检索（TiDB vec_cosine_distance 下推）。
// Serverless 档为库内精确扫描；Dedicated 有 columnar replica 时可建
// HNSW 向量索引（见 0010 迁移注释），同一 SQL 自动加速。
func (s *Store) vectorSearchDB(ctx context.Context, projectID store.UUID, q []float32, k int, gen int64) ([]vecEntry, error) {
	var rows []struct {
		ID         string
		Content    string
		SourceType string
		SourceID   store.UUID
		SourceName string
	}
	err := s.DB.WithContext(ctx).Raw(
		`SELECT id, content, source_type, source_id, source_name
		 FROM rag_chunks
		 WHERE project_id = ? AND generation = ? AND embedding IS NOT NULL
		 ORDER BY vec_cosine_distance(embedding, ?)
		 LIMIT ?`, projectID, gen, VecLiteral(q), k).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]vecEntry, len(rows))
	for i, r := range rows {
		out[i] = vecEntry{
			id: r.ID, content: r.Content, stype: r.SourceType,
			sid: r.SourceID.String(), sname: r.SourceName,
		}
	}
	return out, nil
}

// VecLiteral []float32 → '[f1,f2,…]'（TiDB VECTOR 字面量）。
func VecLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// hexID store.UUID → 32 位无横线 hex（与主库 char(32) 列一致）。
func hexID(u store.UUID) string {
	return strings.ReplaceAll(u.String(), "-", "")
}
