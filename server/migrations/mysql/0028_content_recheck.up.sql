-- 资料变更后的派生内容「待复查」标记。
-- 删除/出新版本时由同一事务确定性写入；是否仍成立由用户逐条确认，不自动改写。
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
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_recheck_mark` (`target_type`,`target_id`,`trigger_kind`,`trigger_id`,`resolved_seq`),
  KEY `ix_recheck_project` (`project_id`,`resolved_seq`),
  KEY `ix_recheck_artifact` (`artifact_id`,`revision`,`resolved_seq`),
  KEY `fk_recheck_resolver` (`resolved_by`),
  CONSTRAINT `fk_recheck_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_recheck_resolver` FOREIGN KEY (`resolved_by`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- wiki 来源回溯写入的唯一键（Run 级近似，重复写入幂等）。
-- MySQL 唯一键对 NULL 不去重；写入侧始终给非空 source_version/chunk_id。
ALTER TABLE `agent_wiki_sources`
  ADD UNIQUE KEY `uq_wiki_source_trace` (`wiki_page_id`,`source_type`,`source_id`,`source_version`,`chunk_id`);
