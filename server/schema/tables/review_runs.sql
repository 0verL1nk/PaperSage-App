-- review_runs 表结构（MySQL/TiDB 方言，真相源）
-- 一次跨章节检查 = 一个 run；报告落在 research_artifacts(kind=chapter_review)。
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
