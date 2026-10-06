-- agent_runs 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 一次 Agent 执行。Run 独立于 HTTP 连接存在：可查询、可取消、可恢复。
-- parent_run_id 形成 Run 树——子 agent（如 search_references）挂成子 Run，
-- 但用户始终只面对一个 Assistant（子 agent 不作为顶层发言者）。
-- 模型与提示版本在此快照：验收要求统计工具调用与计算量，且不能静默换模型。

CREATE TABLE `agent_runs` (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `turn_id` char(32) DEFAULT NULL,
  `parent_run_id` char(32) DEFAULT NULL,
  `agent_key` varchar(64) NOT NULL DEFAULT 'main',
  `status` varchar(20) NOT NULL DEFAULT 'running',
  `output_message_id` char(32) DEFAULT NULL,
  `model_provider` varchar(64) NOT NULL DEFAULT '',
  `model_name` varchar(128) NOT NULL DEFAULT '',
  `prompt_version` varchar(32) NOT NULL DEFAULT '',
  `temperature` decimal(4,2) DEFAULT NULL,
  `input_tokens` int NOT NULL DEFAULT '0',
  `output_tokens` int NOT NULL DEFAULT '0',
  `latency_ms` bigint NOT NULL DEFAULT '0',
  `last_sequence` bigint NOT NULL DEFAULT '0',
  `error_json` json DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  KEY `ix_runs_conversation_started` (`conversation_id`,`started_at`),
  KEY `ix_runs_turn` (`turn_id`),
  KEY `ix_runs_parent` (`parent_run_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_run_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_run_parent` FOREIGN KEY (`parent_run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
