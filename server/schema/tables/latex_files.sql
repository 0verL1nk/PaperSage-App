-- latex_files 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `latex_files` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `file_path` varchar(500) NOT NULL,
  `content_type` varchar(100) DEFAULT NULL,
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `content_hash` varchar(64) DEFAULT NULL,
  `s3_key` varchar(500) DEFAULT NULL,
  `last_saved_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_project_file_path` (`project_id`,`file_path`),
  KEY `ix_latex_files_project_id` (`project_id`),
  KEY `ix_latex_files_content_hash` (`content_hash`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_lf_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
