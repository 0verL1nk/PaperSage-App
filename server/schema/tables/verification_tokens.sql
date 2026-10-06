-- verification_tokens 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `verification_tokens` (
  `token_hash` varchar(64) NOT NULL,
  `user_id` char(32) NOT NULL,
  `expires_at` datetime(6) NOT NULL,
  `used_at` datetime(6) DEFAULT NULL,
  KEY `ix_vt_user` (`user_id`),
  PRIMARY KEY (`token_hash`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_vt_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
