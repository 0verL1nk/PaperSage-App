-- 数据修复不可逆：修复后无法区分哪些 ref_id 原本是文档 ID，不能凭空还原。
-- 需要回退时从备份恢复 research_artifact_dependencies。保留空操作以维持回滚链完整。
SELECT 1;
