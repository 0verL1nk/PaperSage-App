-- agent_events 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 执行事实记录（内部）：断线恢复、审计与用量统计的来源。
-- 传输层对外用的是 AI SDK UI Message Stream Protocol（无 sequence），
-- sequence 留在这里，使服务端将来可平滑切到「增量补拉」。
-- 事件是事实，允许 payload 冗余，不进一步规范化。

CREATE TABLE `agent_events` (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `sequence` bigint NOT NULL,
  `event_type` varchar(48) NOT NULL,
  `entity_type` varchar(32) DEFAULT NULL,
  `entity_id` char(32) DEFAULT NULL,
  `payload_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_event_run_seq` (`run_id`,`sequence`),
  KEY `ix_event_conversation` (`conversation_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_event_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
