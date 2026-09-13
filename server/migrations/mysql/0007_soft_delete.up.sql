-- 0007：全表软删除（gorm.DeletedAt 约定，MySQL 方言）
ALTER TABLE users ADD COLUMN deleted_at datetime(6);
ALTER TABLE project_groups ADD COLUMN deleted_at datetime(6);
ALTER TABLE folders ADD COLUMN deleted_at datetime(6);
ALTER TABLE documents ADD COLUMN deleted_at datetime(6);
ALTER TABLE latex_files ADD COLUMN deleted_at datetime(6);
ALTER TABLE compile_jobs ADD COLUMN deleted_at datetime(6);
ALTER TABLE user_settings ADD COLUMN deleted_at datetime(6);
ALTER TABLE refresh_tokens ADD COLUMN deleted_at datetime(6);
ALTER TABLE verification_tokens ADD COLUMN deleted_at datetime(6);
ALTER TABLE system_settings ADD COLUMN deleted_at datetime(6);
ALTER TABLE agent_conversations ADD COLUMN deleted_at datetime(6);
ALTER TABLE agent_messages ADD COLUMN deleted_at datetime(6);
ALTER TABLE rag_chunks ADD COLUMN deleted_at datetime(6);
ALTER TABLE agent_wiki_pages ADD COLUMN deleted_at datetime(6);
