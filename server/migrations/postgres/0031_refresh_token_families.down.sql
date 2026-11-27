-- 回滚会丢失 family 链数据（不影响登录态本身）；须先回滚二进制再执行。
DROP INDEX IF EXISTS ix_refresh_tokens_family_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS replaced_by;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS family_id;
