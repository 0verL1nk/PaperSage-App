-- compile_jobs 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `compile_jobs` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `user_id` char(32) NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `root_file` varchar(500) NOT NULL DEFAULT 'main.tex',
  `compiler` varchar(20) NOT NULL DEFAULT 'pdflatex',
  `build_id` varchar(100) DEFAULT NULL,
  `error` text DEFAULT NULL,
  `logs` text DEFAULT NULL,
  `sync_state` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_compile_jobs_project_id` (`project_id`),
  KEY `ix_compile_jobs_user_id` (`user_id`),
  KEY `ix_compile_jobs_status` (`status`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_cj_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_cj_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
