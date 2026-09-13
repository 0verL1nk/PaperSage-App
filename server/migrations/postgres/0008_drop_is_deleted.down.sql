ALTER TABLE projects ADD COLUMN IF NOT EXISTS is_deleted boolean NOT NULL DEFAULT false;
-- down 无法恢复删除前的数据，仅恢复形状
