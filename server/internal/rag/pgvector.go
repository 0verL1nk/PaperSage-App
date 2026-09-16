package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PgStore — Neon(pgvector) 后端：chunk 读写与向量检索全部下推 PG。
//
// 与主库解耦（TiDB 管业务表，Neon 管 RAG）：BM25 的内容源也走 Neon
// （通过 RowsForProject 回调喂给内存倒排，主库路径不变）。
type PgStore struct {
	conn *pgx.Conn
}

// NewPgStore 连接 Neon（连接串带 sslmode=require）。
func NewPgStore(ctx context.Context, dsn string) (*PgStore, error) {
	if !strings.Contains(dsn, "sslmode=") {
		dsn += "?sslmode=require"
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgvector connect: %w", err)
	}
	return &PgStore{conn: conn}, nil
}

func (p *PgStore) Close(ctx context.Context) error { return p.conn.Close(ctx) }

// IndexSource 写路径：删旧 → 批量插入（embedding 为 '[f1,f2,…]' 字面量）。
// projectID/sourceID 传入无横线 32 位 hex（与主库存储形态一致）。
func (p *PgStore) IndexSource(ctx context.Context, projectID string,
	sourceType, sourceID, sourceName string, chunks []Chunk, vecs [][]float32) error {

	if _, err := p.conn.Exec(ctx,
		`DELETE FROM rag_chunks WHERE project_id=$1 AND source_type=$2 AND source_id=$3`,
		projectID, sourceType, sourceID); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	// 批量 INSERT（不用 CopyFrom：pgx 对 vector 类型的二进制 COPY 协议
	// 与 pgvector 解析不兼容；显式 ::vector 转换的 INSERT 实测可用）
	b := &strings.Builder{}
	b.WriteString(`INSERT INTO rag_chunks
		(id, project_id, source_type, source_id, source_name, chunk_idx, content, embedding) VALUES `)
	args := make([]any, 0, len(chunks)*8)
	for i, c := range chunks {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 8
		fmt.Fprintf(b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d::vector)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
		args = append(args,
			fmt.Sprintf("%s:%s:%d", sourceType, sourceID, c.Idx),
			projectID, sourceType, sourceID, sourceName,
			c.Idx, c.Content, VecLiteral(vecs[i]))
	}
	_, err := p.conn.Exec(ctx, b.String(), args...)
	return err
}

// DeleteSource 删一个来源的全部块。
func (p *PgStore) DeleteSource(ctx context.Context, projectID, sourceType, sourceID string) error {
	_, err := p.conn.Exec(ctx,
		`DELETE FROM rag_chunks WHERE project_id=$1 AND source_type=$2 AND source_id=$3`,
		projectID, sourceType, sourceID)
	return err
}

// SearchVector 向量检索下推：ORDER BY embedding <=> $query LIMIT k
// （HNSW 索引命中，pgvector 余弦距离）。
func (p *PgStore) SearchVector(ctx context.Context, projectID string, q []float32, k int) ([]PgRow, error) {
	rows, err := p.conn.Query(ctx, `
		SELECT id, source_type, source_id, source_name, content,
		       1 - (embedding <=> $2::vector) AS score
		FROM rag_chunks
		WHERE project_id = $1
		ORDER BY embedding <=> $2::vector
		LIMIT $3`, projectID, VecLiteral(q), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgRow
	for rows.Next() {
		var r PgRow
		if err := rows.Scan(&r.ID, &r.SourceType, &r.SourceID, &r.SourceName, &r.Content, &r.Score); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PgRow Neon 检索返回的行（source_id 为 hex 文本，与主库 IndexedChunk 同形）。
type PgRow struct {
	ID         string
	SourceType string
	SourceID   string
	SourceName string
	ChunkIdx   int
	Content    string
	Score      float64
}

// SearchBM25 内容行（喂内存倒排）。
func (p *PgStore) SearchBM25(ctx context.Context, projectID string) ([]PgRow, error) {
	rows, err := p.conn.Query(ctx, `
		SELECT id, source_type, source_id, source_name, chunk_idx, content
		FROM rag_chunks WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgRow
	for rows.Next() {
		var r PgRow
		if err := rows.Scan(&r.ID, &r.SourceType, &r.SourceID, &r.SourceName, &r.ChunkIdx, &r.Content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
