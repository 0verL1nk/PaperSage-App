-- research_artifacts 表结构（MySQL/TiDB 方言，真相源）
-- 组织产物聚合根：固定 kind、当前修订号与已采用修订；正文在不可变修订里。

CREATE TABLE `research_artifacts` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL,
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `current_revision` int NOT NULL DEFAULT '0',
  `adopted_revision` int DEFAULT NULL,
  `client_request_id` varchar(64) DEFAULT NULL,
  `created_by` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  `deleted_seq` bigint NOT NULL DEFAULT '0',
  UNIQUE KEY `uq_research_artifact_request` (`project_id`,`client_request_id`,`deleted_seq`),
  KEY `ix_research_artifacts_project_kind` (`project_id`,`kind`,`deleted_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_artifact_creator` (`created_by`),
  CONSTRAINT `fk_artifact_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_artifact_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
