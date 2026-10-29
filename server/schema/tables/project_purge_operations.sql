-- project_purge_operations 表结构（MySQL/TiDB 方言，真相源）
-- 持久硬删除意图；项目外部数据清完前不删除该行或项目元数据。

CREATE TABLE `project_purge_operations` (
  `project_id` char(32) NOT NULL,
  `owner_id` char(32) NOT NULL,
  `attempt_count` bigint NOT NULL DEFAULT 0,
  `documents_offset` bigint NOT NULL DEFAULT 0,
  `latex_files_offset` bigint NOT NULL DEFAULT 0,
  `artifacts_offset` bigint NOT NULL DEFAULT 0,
  `s3_done` boolean NOT NULL DEFAULT false,
  `compact_prefix_done` boolean NOT NULL DEFAULT false,
  `dashed_prefix_done` boolean NOT NULL DEFAULT false,
  `rag_done` boolean NOT NULL DEFAULT false,
  `lease_token` char(36) DEFAULT NULL,
  `lease_until` datetime(6) DEFAULT NULL,
  `last_error` varchar(120) NOT NULL DEFAULT '',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`project_id`) /*T![clustered_index] CLUSTERED */,
  KEY `ix_project_purge_owner` (`owner_id`),
  KEY `ix_project_purge_lease` (`lease_until`),
  CONSTRAINT `fk_project_purge_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
