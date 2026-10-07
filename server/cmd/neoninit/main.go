// Command neoninit — Neon(pgvector) 初始化：扩展 + rag_chunks 表 + HNSW 索引。
// 一次性工具，NEON_DATABASE_URL 环境变量提供连接串。
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("NEON_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "neoninit: NEON_DATABASE_URL required")
		os.Exit(1)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		fatal("ping: %v", err)
	}
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`DROP TABLE IF EXISTS rag_chunks`,
		`CREATE TABLE IF NOT EXISTS rag_chunks (
			id          varchar(80) PRIMARY KEY,
			project_id  varchar(32) NOT NULL,
			source_type varchar(20) NOT NULL DEFAULT 'document',
			source_id   varchar(32) NOT NULL,
			source_name varchar(255) NOT NULL DEFAULT '',
			chunk_idx   int NOT NULL DEFAULT 0,
			content     text NOT NULL,
			embedding   vector(1024),
			generation  bigint NOT NULL DEFAULT 1,
			created_at  timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS ix_rag_project ON rag_chunks (project_id)`,
		`CREATE INDEX IF NOT EXISTS ix_rag_source ON rag_chunks (source_type, source_id)`,
		`CREATE INDEX IF NOT EXISTS ix_rag_generation ON rag_chunks (project_id, generation)`,
		`CREATE INDEX IF NOT EXISTS ix_rag_embedding_hnsw ON rag_chunks USING hnsw (embedding vector_cosine_ops)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			fatal("exec: %v\nSQL: %.80s", err, s)
		}
	}
	fmt.Println("neoninit: OK (vector 扩展 + rag_chunks + HNSW)")
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "neoninit: "+f+"\n", a...)
	os.Exit(1)
}
