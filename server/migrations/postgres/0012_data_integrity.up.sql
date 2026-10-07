-- 0012：数据完整性重构（openspec/changes/data-integrity-refactor）
--
-- 1) latex_files 唯一键纳入 deleted_seq——删除后同名重建不再撞唯一键
-- 2) 移除「有列无模型」的 deleted_at（派生索引与基础设施表，软删语义本就不适用）
-- 3) agent_wiki_pages 补 meta（Go 模型早已在写，DDL 一直没有该列）
-- 4) rag_chunks.generation + rag_index_generations（索引代次）
-- 5) 会话与消息的查询索引
--
-- 本迁移只有形状变化，无破坏性数据变更；回滚见 .down.sql（无法还原已删列的数据）。

-- ── 1. latex_files：唯一键纳入 deleted_seq ──────────────────────────
ALTER TABLE latex_files ADD COLUMN IF NOT EXISTS deleted_seq bigint NOT NULL DEFAULT 0;
-- 存量已软删行回填单调非零值，语义与 SoftDeleteTime 一致（unix 微秒）。
-- 旧唯一键 (project_id, file_path) 保证每个路径至多一行，故不会撞新唯一键。
UPDATE latex_files
   SET deleted_seq = (EXTRACT(EPOCH FROM deleted_at) * 1000000)::bigint
 WHERE deleted_at IS NOT NULL AND deleted_seq = 0;
ALTER TABLE latex_files DROP CONSTRAINT IF EXISTS uq_project_file_path;
ALTER TABLE latex_files ADD CONSTRAINT uq_project_file_path
    UNIQUE (project_id, file_path, deleted_seq);

-- ── 2. 移除不承重的 deleted_at ──────────────────────────────────────
-- 这些表要么是派生索引（可由来源重建），要么是基础设施记录（无恢复需求），
-- 且 Go 模型从未映射 deleted_at——留着只会让"已删除"的判定悬空。
ALTER TABLE refresh_tokens      DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE verification_tokens DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE system_settings     DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE user_settings       DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE rag_chunks          DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE agent_wiki_pages    DROP COLUMN IF EXISTS deleted_at;

-- ── 3. agent_wiki_pages.meta ────────────────────────────────────────
ALTER TABLE agent_wiki_pages ADD COLUMN IF NOT EXISTS meta json;

-- ── 4. 索引代次 ─────────────────────────────────────────────────────
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS generation bigint NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS ix_rag_chunks_generation ON rag_chunks (project_id, generation);

CREATE TABLE IF NOT EXISTS rag_index_generations (
    id              uuid         PRIMARY KEY,
    project_id      uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    generation      bigint       NOT NULL,
    status          varchar(20)  NOT NULL DEFAULT 'building',
    embedding_model varchar(128) NOT NULL DEFAULT '',
    embedding_dim   int          NOT NULL DEFAULT 0,
    chunk_count     int          NOT NULL DEFAULT 0,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    activated_at    timestamptz,
    UNIQUE (project_id, generation)
);
CREATE INDEX IF NOT EXISTS ix_rag_generation_status
    ON rag_index_generations (project_id, status);

-- 存量项目补第 1 代活动代次，使既有片段（generation 默认 1）继续可检索。
-- id 由 project_id 派生，保证迁移重放时确定且幂等。
INSERT INTO rag_index_generations (id, project_id, generation, status, activated_at)
SELECT md5(p.id::text || ':1')::uuid, p.id, 1, 'active', now()
  FROM projects p
 WHERE NOT EXISTS (
       SELECT 1 FROM rag_index_generations g WHERE g.project_id = p.id
 );

-- ── 5. 查询索引 ─────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS ix_agent_conversations_list
    ON agent_conversations (project_id, user_id, updated_at);
CREATE INDEX IF NOT EXISTS ix_agent_messages_history
    ON agent_messages (conversation_id, created_at);
