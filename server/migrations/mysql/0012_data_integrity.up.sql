-- 0012：数据完整性重构（openspec/changes/data-integrity-refactor）
-- 见 postgres/0012_data_integrity.up.sql 的完整说明。
-- 本树不写 IF EXISTS 守卫（golang-migrate 每版本只跑一次；MySQL 8 不支持索引级 IF EXISTS）。

-- ── 1. latex_files：唯一键纳入 deleted_seq ──────────────────────────
ALTER TABLE latex_files ADD COLUMN deleted_seq bigint NOT NULL DEFAULT 0;
UPDATE latex_files
   SET deleted_seq = UNIX_TIMESTAMP(deleted_at) * 1000000
 WHERE deleted_at IS NOT NULL AND deleted_seq = 0;
ALTER TABLE latex_files DROP INDEX uq_project_file_path;
ALTER TABLE latex_files ADD UNIQUE KEY uq_project_file_path (project_id, file_path, deleted_seq);

-- ── 2. 移除不承重的 deleted_at ──────────────────────────────────────
ALTER TABLE refresh_tokens      DROP COLUMN deleted_at;
ALTER TABLE verification_tokens DROP COLUMN deleted_at;
ALTER TABLE system_settings     DROP COLUMN deleted_at;
ALTER TABLE user_settings       DROP COLUMN deleted_at;
ALTER TABLE rag_chunks          DROP COLUMN deleted_at;
ALTER TABLE agent_wiki_pages    DROP COLUMN deleted_at;

-- ── 3. agent_wiki_pages.meta ────────────────────────────────────────
ALTER TABLE agent_wiki_pages ADD COLUMN meta json;

-- ── 4. 索引代次 ─────────────────────────────────────────────────────
ALTER TABLE rag_chunks ADD COLUMN generation bigint NOT NULL DEFAULT 1;
CREATE INDEX ix_rag_chunks_generation ON rag_chunks (project_id, generation);

CREATE TABLE rag_index_generations (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `generation` bigint NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'building',
  `embedding_model` varchar(128) NOT NULL DEFAULT '',
  `embedding_dim` int NOT NULL DEFAULT '0',
  `chunk_count` int NOT NULL DEFAULT '0',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `activated_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_rag_generation` (`project_id`,`generation`),
  KEY `ix_rag_generation_status` (`project_id`,`status`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rg_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- 存量项目补第 1 代活动代次（id 由 project_id 派生，重放确定且幂等）
INSERT INTO rag_index_generations (id, project_id, generation, status, activated_at)
SELECT MD5(CONCAT(p.id, ':1')), p.id, 1, 'active', NOW(6)
  FROM projects p
 WHERE NOT EXISTS (
       SELECT 1 FROM rag_index_generations g WHERE g.project_id = p.id
 );

-- ── 5. 查询索引 ─────────────────────────────────────────────────────
CREATE INDEX ix_agent_conversations_list ON agent_conversations (project_id, user_id, updated_at);
CREATE INDEX ix_agent_messages_history ON agent_messages (conversation_id, created_at);
