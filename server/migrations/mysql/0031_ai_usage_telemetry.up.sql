-- AI 调用计量（MySQL/TiDB 方言）与 Run 的功能入口快照。
-- 删除 agent_runs 的 input_tokens/output_tokens/latency_ms：全仓从未写入。

CREATE TABLE `ai_model_calls` (
  `id` char(32) NOT NULL,
  `user_id` char(32) DEFAULT NULL,
  `project_id` char(32) DEFAULT NULL,
  `run_id` char(32) DEFAULT NULL,
  `feature_kind` varchar(32) NOT NULL,
  `provider` varchar(64) NOT NULL,
  `provider_source` varchar(20) NOT NULL,
  `model` varchar(128) NOT NULL,
  `is_stream` tinyint(1) NOT NULL DEFAULT 0,
  `status` varchar(16) NOT NULL,
  `error_code` varchar(32) DEFAULT NULL,
  `http_status` smallint DEFAULT NULL,
  `prompt_tokens` int DEFAULT NULL,
  `completion_tokens` int DEFAULT NULL,
  `reasoning_tokens` int DEFAULT NULL,
  `latency_ms` int NOT NULL,
  `first_token_ms` int DEFAULT NULL,
  `created_day` char(10) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `ix_ai_calls_created` (`created_at`),
  KEY `ix_ai_calls_user_created` (`user_id`,`created_at`),
  KEY `ix_ai_calls_project_created` (`project_id`,`created_at`),
  KEY `ix_ai_calls_day_model` (`created_day`,`model`),
  KEY `ix_ai_calls_feature_day` (`feature_kind`,`created_day`),
  KEY `ix_ai_calls_run` (`run_id`),
  CONSTRAINT `fk_ai_calls_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_ai_calls_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE `agent_runs` ADD COLUMN `feature_kind` varchar(32) NOT NULL DEFAULT 'chat';
ALTER TABLE `agent_runs` DROP COLUMN `input_tokens`, DROP COLUMN `output_tokens`, DROP COLUMN `latency_ms`;
