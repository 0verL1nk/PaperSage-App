-- ai_model_calls 表结构（MySQL/TiDB 方言，真相源）
--
-- 一次模型调用一行，只追加、按保留期硬删（成本台账，不随 Run/会话删除）。
-- 隐私边界：不存 prompt/回复正文、API Key、base_url、请求头与原始错误消息；
-- token 列可空——provider 不返回 usage 时为 NULL，不补 0。
-- created_day 在写入时按 AI_USAGE_TIMEZONE 算好，规避三方言日期截断差异。

CREATE TABLE `ai_model_calls` (
  `id` char(32) NOT NULL,
  `user_id` char(32) DEFAULT NULL,
  `project_id` char(32) DEFAULT NULL,
  -- 不建外键：Run 可随会话删除，成本台账要留存。
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
