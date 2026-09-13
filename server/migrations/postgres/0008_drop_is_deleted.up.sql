-- 0008：清理 projects.is_deleted 死列
-- 模型已统一 gorm.DeletedAt 软删除（0007），该列无任何代码读写。
ALTER TABLE projects DROP COLUMN IF EXISTS is_deleted;
