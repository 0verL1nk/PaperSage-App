-- 0005：RAG 索引真相源 + LLM wiki
-- rag_chunks 是 chunk 的持久真相源（向量存 chromem-go 持久库，
-- BM25 内存索引启动后按项目惰性重建）。
CREATE TABLE IF NOT EXISTS rag_chunks (
    id          varchar(80) PRIMARY KEY, -- {doc_id}:{chunk_idx}
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    source_type varchar(20) NOT NULL DEFAULT 'document', -- document | wiki
    source_id   uuid        NOT NULL,
    source_name varchar(255) NOT NULL DEFAULT '',
    chunk_idx   int         NOT NULL DEFAULT 0,
    content     text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_rag_chunks_project ON rag_chunks (project_id);
CREATE INDEX IF NOT EXISTS ix_rag_chunks_source ON rag_chunks (source_type, source_id);

CREATE TABLE IF NOT EXISTS agent_wiki_pages (
    id         uuid PRIMARY KEY,
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    title      varchar(200) NOT NULL,
    content    text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, title)
);
