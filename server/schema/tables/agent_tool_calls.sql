-- agent_tool_calls 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 工具调用从「函数调用」升级为可审计状态机：
--   input_streaming → ready → validating → awaiting_approval
--   → approved / denied → executing → succeeded / failed
-- seq 取代原先"按同名工具队列配对"的脆弱约定（同一轮两次同名调用会串味），
-- idempotency_key 防断线重试导致的副作用重复。

CREATE TABLE `agent_tool_calls` (
  `id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `seq` int NOT NULL DEFAULT '0',
  `tool_name` varchar(64) NOT NULL,
  `state` varchar(24) NOT NULL DEFAULT 'ready',
  `risk_level` varchar(12) NOT NULL DEFAULT 'read',
  `requires_approval` tinyint(1) NOT NULL DEFAULT '0',
  `input_json` json DEFAULT NULL,
  `output_json` json DEFAULT NULL,
  `artifact_id` char(32) DEFAULT NULL,
  `approval_id` varchar(64) DEFAULT NULL,
  `arguments_hash` char(64) DEFAULT NULL,
  `idempotency_key` varchar(128) DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `error_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_tool_run_seq` (`run_id`,`seq`),
  UNIQUE KEY `uq_tool_idempotency` (`idempotency_key`),
  KEY `ix_tool_state` (`state`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_tool_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
