-- rag_chunks 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `rag_chunks` (
  `id` varchar(80) NOT NULL,
  `project_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL DEFAULT 'document',
  `source_id` char(32) NOT NULL,
  `source_name` varchar(255) NOT NULL DEFAULT '',
  `chunk_idx` int NOT NULL DEFAULT '0',
  `content` text NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `embedding` mediumtext DEFAULT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_rag_chunks_project` (`project_id`),
  KEY `ix_rag_chunks_source` (`source_type`,`source_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rc_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
