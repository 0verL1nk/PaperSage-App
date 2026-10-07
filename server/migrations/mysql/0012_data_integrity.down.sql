-- 0012 回滚：仅恢复形状，无法还原被 drop 的 deleted_at 中的数据。
DROP INDEX ix_agent_messages_history ON agent_messages;
DROP INDEX ix_agent_conversations_list ON agent_conversations;

DROP TABLE IF EXISTS rag_index_generations;
DROP INDEX ix_rag_chunks_generation ON rag_chunks;
ALTER TABLE rag_chunks DROP COLUMN generation;

ALTER TABLE agent_wiki_pages DROP COLUMN meta;

ALTER TABLE agent_wiki_pages    ADD COLUMN deleted_at datetime(6) DEFAULT NULL;
ALTER TABLE rag_chunks          ADD COLUMN deleted_at datetime(6) DEFAULT NULL;
ALTER TABLE user_settings       ADD COLUMN deleted_at datetime(6) DEFAULT NULL;
ALTER TABLE system_settings     ADD COLUMN deleted_at datetime(6) DEFAULT NULL;
ALTER TABLE verification_tokens ADD COLUMN deleted_at datetime(6) DEFAULT NULL;
ALTER TABLE refresh_tokens      ADD COLUMN deleted_at datetime(6) DEFAULT NULL;

-- 已软删的重复行在旧 schema 下无法表达（窄唯一键不允许同路径多行），先删除，
-- 否则 ADD UNIQUE KEY 会因 ERROR 1062 失败、迁移卡在 dirty 状态。
DELETE FROM latex_files WHERE deleted_seq > 0;

ALTER TABLE latex_files DROP INDEX uq_project_file_path;
ALTER TABLE latex_files ADD UNIQUE KEY uq_project_file_path (project_id, file_path);
ALTER TABLE latex_files DROP COLUMN deleted_seq;
