-- agent_evidence 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 证据身份与版本：每条被引用的来源记录指向哪个来源的哪个版本、哪一段。
-- source_id 是多态引用（document / latex_file / wiki / chunk），
-- 故 DB 层不设外键，由应用层保证；(source_type, source_id) 建索引。
-- 来源改动或撤权后应据此标记受影响内容（docs/project/12-ai-writing-design.md §8）。

CREATE TABLE `agent_evidence` (
  `id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL,
  `source_id` char(32) NOT NULL,
  `source_version` char(64) DEFAULT NULL,
  `chunk_id` varchar(80) DEFAULT NULL,
  `locator` varchar(255) DEFAULT NULL,
  `quote` text DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_evidence_run` (`run_id`),
  KEY `ix_evidence_source` (`source_type`,`source_id`),
  UNIQUE KEY `uq_evidence_run_chunk_version` (`run_id`,`chunk_id`,`source_version`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_evidence_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
