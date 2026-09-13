-- documents 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `documents` (
  `id` char(32) NOT NULL,
  `name` varchar(255) NOT NULL,
  `type` varchar(100) NOT NULL,
  `size` bigint NOT NULL DEFAULT '0',
  `status` varchar(20) NOT NULL DEFAULT 'uploaded',
  `s3_key` varchar(500) NOT NULL,
  `remark` text DEFAULT NULL,
  `project_id` char(32) NOT NULL,
  `folder_id` char(32) DEFAULT NULL,
  `owner_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_documents_project` (`project_id`),
  KEY `ix_documents_folder` (`folder_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_doc_owner` (`owner_id`),
  CONSTRAINT `fk_doc_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_doc_folder` FOREIGN KEY (`folder_id`) REFERENCES `folders` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_doc_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
