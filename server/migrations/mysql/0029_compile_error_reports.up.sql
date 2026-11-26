-- 浏览器 WASM 编译失败诊断（运维用，admin 可见）。
-- 与 compile_jobs（用户可见的编译历史）职责分离：本表只存脱敏诊断，
-- 严禁论文正文；靠保留期硬删，不做软删。
CREATE TABLE `compile_error_reports` (
  `id` char(32) NOT NULL,
  `user_id` char(32) DEFAULT NULL,
  `project_id` char(32) DEFAULT NULL,
  `compile_job_id` char(32) DEFAULT NULL,
  `is_guest` tinyint(1) NOT NULL DEFAULT 0,
  `client_kind` varchar(16) NOT NULL DEFAULT 'web',
  `engine` varchar(20) NOT NULL,
  `engine_version` varchar(64) DEFAULT NULL,
  `resource_profile` varchar(64) DEFAULT NULL,
  `failure_kind` varchar(16) NOT NULL,
  `failure_stage` varchar(24) NOT NULL,
  `duration_ms` int NOT NULL DEFAULT 0,
  `http_status` smallint DEFAULT NULL,
  `error_message` varchar(512) NOT NULL,
  `log_excerpt` text DEFAULT NULL,
  `failed_resources` json DEFAULT NULL,
  `client_env` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `ix_cer_created` (`created_at`),
  KEY `ix_cer_user` (`user_id`,`created_at`),
  KEY `ix_cer_project` (`project_id`,`created_at`),
  KEY `ix_cer_kind` (`failure_kind`,`created_at`),
  KEY `ix_cer_engine` (`engine`,`created_at`),
  CONSTRAINT `fk_cer_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_cer_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
