-- 回滚会丢失 family 链数据（不影响登录态本身）；须先回滚二进制再执行。
ALTER TABLE `refresh_tokens`
  DROP INDEX `ix_refresh_tokens_family_id`,
  DROP COLUMN `replaced_by`,
  DROP COLUMN `family_id`;
