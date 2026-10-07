-- agent_artifacts 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 工具的大结果（文献全文、报表等）不进消息与模型上下文，只留摘要 + 引用。

CREATE TABLE `agent_artifacts` (
  `id` char(32) NOT NULL,
  `run_id` char(32) DEFAULT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL DEFAULT 'text',
  `s3_key` varchar(500) NOT NULL,
  `mime` varchar(128) NOT NULL DEFAULT 'text/plain',
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `summary_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_artifact_run` (`run_id`),
  KEY `ix_artifact_project` (`project_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_artifact_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_artifact_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
