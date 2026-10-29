CREATE TABLE `latex_object_cleanups` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `object_key` varchar(500) NOT NULL,
  `attempts` bigint NOT NULL DEFAULT 0,
  `last_error` varchar(32) NOT NULL DEFAULT '',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_latex_object_cleanup_key` (`object_key`),
  KEY `ix_latex_object_cleanup_project_created` (`project_id`,`created_at`),
  CONSTRAINT `fk_latex_object_cleanup_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
