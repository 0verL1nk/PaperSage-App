-- refresh_tokens 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `refresh_tokens` (
  `token_hash` varchar(64) NOT NULL,
  `user_id` char(32) NOT NULL,
  `expires_at` datetime(6) NOT NULL,
  `revoked_at` datetime(6) DEFAULT NULL,
  -- 一次登录 = 一个 family；轮换继承，重放整族吊销。可空兼容历史行。
  `family_id` char(32) DEFAULT NULL,
  -- 轮换产生的新 token 的 SHA-256；非空表示「被轮换」而非「被登出」。
  `replaced_by` varchar(64) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_refresh_tokens_user_id` (`user_id`),
  KEY `ix_refresh_tokens_family_id` (`family_id`),
  PRIMARY KEY (`token_hash`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rt_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
