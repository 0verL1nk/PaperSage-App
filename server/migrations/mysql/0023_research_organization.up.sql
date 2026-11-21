CREATE TABLE `research_artifacts` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL,
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `current_revision` int NOT NULL DEFAULT 0,
  `adopted_revision` int DEFAULT NULL,
  `client_request_id` varchar(64) DEFAULT NULL,
  `created_by` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  `deleted_seq` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_research_artifact_request` (`project_id`,`client_request_id`,`deleted_seq`),
  KEY `ix_research_artifacts_project_kind` (`project_id`,`kind`,`deleted_at`),
  KEY `fk_artifact_creator` (`created_by`),
  CONSTRAINT `fk_artifact_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_artifact_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `research_artifact_revisions` (
  `id` char(32) NOT NULL,
  `artifact_id` char(32) NOT NULL,
  `revision` int NOT NULL,
  `payload_json` json NOT NULL,
  `origin` varchar(16) NOT NULL DEFAULT 'user',
  `status` varchar(20) NOT NULL DEFAULT 'candidate',
  `run_id` char(32) DEFAULT NULL,
  `created_by` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_artifact_revision` (`artifact_id`,`revision`),
  KEY `ix_artifact_revision_status` (`artifact_id`,`status`),
  KEY `fk_revision_run` (`run_id`),
  KEY `fk_revision_creator` (`created_by`),
  CONSTRAINT `fk_revision_artifact` FOREIGN KEY (`artifact_id`) REFERENCES `research_artifacts` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_revision_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_revision_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `research_artifact_dependencies` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `artifact_id` char(32) NOT NULL,
  `revision` int NOT NULL,
  `kind` varchar(24) NOT NULL,
  `ref_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_artifact_dependency` (`artifact_id`,`revision`,`kind`,`ref_id`),
  KEY `ix_artifact_dependency_ref` (`project_id`,`kind`,`ref_id`),
  CONSTRAINT `fk_dependency_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_dependency_artifact` FOREIGN KEY (`artifact_id`) REFERENCES `research_artifacts` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
