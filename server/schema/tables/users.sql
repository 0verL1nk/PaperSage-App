-- users 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

-- 已知限制：本表两个唯一键（username / email）都不含 deleted_seq，
-- 因此账号被软删后同名/同邮箱无法再注册。当前代码库没有任何删除用户的
-- 路径，该问题不会被触发；待用户注销功能立项时一并解决。
-- 之所以不现在修：GORM 的 uniqueIndex tag 中一个字段只能属于一个索引，
-- 无法让 deleted_seq 同时加入这两个索引，AutoMigrate（SQLite）表达不了。
-- 详见 openspec/changes/data-integrity-refactor/design.md D1。
CREATE TABLE `users` (
  `id` char(32) NOT NULL,
  `username` varchar(50) NOT NULL,
  `password_hash` varchar(255) NOT NULL,
  `email` varchar(255) DEFAULT NULL,
  `email_verified` tinyint(1) NOT NULL DEFAULT '0',
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `is_superuser` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `is_email_registered` tinyint(1) NOT NULL DEFAULT '0',
  `auth_version` bigint NOT NULL DEFAULT '0',
  `deleted_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `ix_users_username` (`username`),
  UNIQUE KEY `ix_users_email` (`email`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
