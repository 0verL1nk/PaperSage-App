-- 0007：全表软删除（gorm.DeletedAt 约定）
-- BaseModel 增加 DeletedAt → 所有业务表带 delete time；
-- 查询自动过滤已删行，Delete() 落 UPDATE 而非物理删除。
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE project_groups ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
-- projects 已有 deleted_at（原自定义软删），改为 gorm 约定共用
ALTER TABLE folders ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE latex_files ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE compile_jobs ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE verification_tokens ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE agent_conversations ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE agent_wiki_pages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
