-- 0008：清理 projects.is_deleted 死列（MySQL/TiDB 方言）
ALTER TABLE projects DROP COLUMN is_deleted;
