-- email_login_codes 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

-- 验证码登录的一次性凭据：只存 bcrypt 哈希（自带盐），无明文。
-- 不绑 user_id——首次登录时用户还不存在；按 (email, created_at) 取最新未消费行。
-- 基础设施表硬删（与 verification_tokens / refresh_tokens 同语义）。
CREATE TABLE `email_login_codes` (
  `id` char(32) NOT NULL,
  `email` varchar(255) NOT NULL,
  `code_hash` varchar(255) NOT NULL,
  `attempts` int NOT NULL DEFAULT '0',
  `expires_at` datetime(6) NOT NULL,
  `used_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_email_login_codes_email` (`email`, `created_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
