-- research_artifact_revisions 表结构（MySQL/TiDB 方言，真相源）
-- 不可变修订：payload_json 是经服务端类型校验的领域 JSON；历史不得改写。

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
  UNIQUE KEY `uq_artifact_revision` (`artifact_id`,`revision`),
  KEY `ix_artifact_revision_status` (`artifact_id`,`status`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_revision_run` (`run_id`),
  KEY `fk_revision_creator` (`created_by`),
  CONSTRAINT `fk_revision_artifact` FOREIGN KEY (`artifact_id`) REFERENCES `research_artifacts` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_revision_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_revision_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
