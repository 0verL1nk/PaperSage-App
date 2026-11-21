-- The matching version and revision projection must be published atomically.
CREATE TABLE `project_sources` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL,
  `current_revision` bigint NOT NULL DEFAULT 0,
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_project_sources_id_project` (`id`,`project_id`),
  KEY `ix_project_sources_project` (`project_id`),
  CONSTRAINT `ck_project_sources_revision` CHECK (`current_revision` >= 0),
  CONSTRAINT `fk_project_sources_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
