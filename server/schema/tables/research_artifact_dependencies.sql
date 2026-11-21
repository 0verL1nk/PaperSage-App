-- research_artifact_dependencies 表结构（MySQL/TiDB 方言，真相源）
-- 修订实际依赖：来源版本/段落、生成 Run、被引用的产物修订；用于复查与失效。

CREATE TABLE `research_artifact_dependencies` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `artifact_id` char(32) NOT NULL,
  `revision` int NOT NULL,
  `kind` varchar(24) NOT NULL,
  `ref_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_artifact_dependency` (`artifact_id`,`revision`,`kind`,`ref_id`),
  KEY `ix_artifact_dependency_ref` (`project_id`,`kind`,`ref_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_dependency_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_dependency_artifact` FOREIGN KEY (`artifact_id`) REFERENCES `research_artifacts` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
