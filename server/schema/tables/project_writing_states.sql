-- 项目写作状态（MySQL/TiDB 方言真相源）
CREATE TABLE `project_writing_states` (
  `project_id` char(32) NOT NULL,
  `brief` json NOT NULL,
  `requirements` json NOT NULL,
  `phases` json NOT NULL,
  `current_phase` varchar(20) NOT NULL DEFAULT 'prepare',
  `revision` int NOT NULL DEFAULT 1,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`project_id`) /*T![clustered_index] CLUSTERED */,
  KEY `ix_project_writing_states_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_writing_state_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
