-- 0012 回滚：仅恢复形状，无法还原被 drop 的 deleted_at 中的数据。
DROP INDEX IF EXISTS ix_agent_messages_history;
DROP INDEX IF EXISTS ix_agent_conversations_list;

DROP TABLE IF EXISTS rag_index_generations;
DROP INDEX IF EXISTS ix_rag_chunks_generation;
ALTER TABLE rag_chunks DROP COLUMN IF EXISTS generation;

ALTER TABLE agent_wiki_pages DROP COLUMN IF EXISTS meta;

ALTER TABLE agent_wiki_pages    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE rag_chunks          ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE user_settings       ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE system_settings     ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE verification_tokens ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE refresh_tokens      ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- 回滚只能恢复形状。已软删的重复行在旧 schema 下无法表达：窄唯一键
-- UNIQUE(project_id, file_path) 不允许同路径多行，故必须先删除它们，
-- 否则 ADD CONSTRAINT 会因重复键失败、迁移卡在 dirty 状态。
DELETE FROM latex_files WHERE deleted_seq > 0;

ALTER TABLE latex_files DROP CONSTRAINT IF EXISTS uq_project_file_path;
ALTER TABLE latex_files ADD CONSTRAINT uq_project_file_path UNIQUE (project_id, file_path);
ALTER TABLE latex_files DROP COLUMN IF EXISTS deleted_seq;
