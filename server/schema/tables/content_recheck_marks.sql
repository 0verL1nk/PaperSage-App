-- content_recheck_marks 表结构（MySQL/TiDB 方言，真相源）
-- 资料删除/出新版本后，依赖它的产物修订、证据与 wiki 页在这里挂「待复查」。
-- 只做确定性依赖标记，是否仍成立由用户逐条确认；不自动改写任何内容。
-- resolved_seq 沿用 deleted_seq 模式：未处理为 0，处理时写 unix 微秒，
-- 使同一目标同一触发在处理后可再次触发而不撞唯一键。

CREATE TABLE `content_recheck_marks` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `target_type` varchar(24) NOT NULL,
  `target_id` char(32) NOT NULL,
  `artifact_id` char(32) DEFAULT NULL,
  `revision` int DEFAULT NULL,
  `trigger_kind` varchar(24) NOT NULL,
  `trigger_id` char(32) NOT NULL,
  `reason` varchar(500) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `resolved_at` datetime(6) DEFAULT NULL,
  `resolved_by` char(32) DEFAULT NULL,
  `resolution` varchar(16) DEFAULT NULL,
  `resolved_seq` bigint NOT NULL DEFAULT 0,
  UNIQUE KEY `uq_recheck_mark` (`target_type`,`target_id`,`trigger_kind`,`trigger_id`,`resolved_seq`),
  KEY `ix_recheck_project` (`project_id`,`resolved_seq`),
  KEY `ix_recheck_artifact` (`artifact_id`,`revision`,`resolved_seq`),
  KEY `fk_recheck_resolver` (`resolved_by`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_recheck_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_recheck_resolver` FOREIGN KEY (`resolved_by`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
