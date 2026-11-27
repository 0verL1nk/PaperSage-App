-- 跨章节检查任务（MySQL/TiDB 方言）。报告仍落在 research_artifacts。
ALTER TABLE `agent_conversations` ADD COLUMN `kind` varchar(20) NOT NULL DEFAULT 'chat';
CREATE INDEX `ix_agent_conversations_kind` ON `agent_conversations` (`project_id`,`user_id`,`kind`,`updated_at`);

CREATE TABLE `review_runs` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `user_id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `client_request_id` varchar(64) NOT NULL,
  `status` varchar(20) NOT NULL,
  `scope_json` json NOT NULL,
  `incremental` tinyint(1) NOT NULL DEFAULT 0,
  `model_provider` varchar(64) NOT NULL DEFAULT '',
  `model_name` varchar(128) NOT NULL DEFAULT '',
  `prompt_version` varchar(32) NOT NULL DEFAULT '',
  `artifact_id` char(32) DEFAULT NULL,
  `artifact_revision` int DEFAULT NULL,
  `error_text` varchar(512) NOT NULL DEFAULT '',
  `cancel_requested_at` datetime(6) DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_review_runs_client_request` (`project_id`,`client_request_id`),
  KEY `ix_review_runs_project_created` (`project_id`,`created_at`),
  KEY `ix_review_runs_status` (`status`),
  CONSTRAINT `fk_review_runs_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_review_runs_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `review_units` (
  `id` char(32) NOT NULL,
  `review_run_id` char(32) NOT NULL,
  `seq` int NOT NULL,
  `kind` varchar(16) NOT NULL,
  `label` varchar(200) NOT NULL,
  `file_paths_json` json NOT NULL,
  `section_ids_json` json NOT NULL,
  `dimensions_json` json NOT NULL,
  `context_json` json NOT NULL,
  `input_fingerprint` char(64) NOT NULL,
  `status` varchar(16) NOT NULL,
  `agent_run_id` char(32) DEFAULT NULL,
  `reused_from_unit_id` char(32) DEFAULT NULL,
  `findings_json` json NOT NULL,
  `dimension_results_json` json NOT NULL,
  `sources_read_json` json NOT NULL,
  `error_text` varchar(512) NOT NULL DEFAULT '',
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_review_units_seq` (`review_run_id`,`seq`),
  KEY `ix_review_units_fingerprint` (`input_fingerprint`,`status`),
  CONSTRAINT `fk_review_units_run` FOREIGN KEY (`review_run_id`) REFERENCES `review_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
