-- agent_change_proposals 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 助手对稿件的修改建议。助手**不写盘**：建议由用户在前端确认后，
-- 由服务端重新校验权限与基准版本再执行替换。
-- base_content_hash 是产出建议时的目标文件版本——稿件一变，旧建议即失效（stale），
-- 以免用旧基准覆盖新内容（docs/project/12-ai-writing-design.md §7）。

CREATE TABLE `agent_change_proposals` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `run_id` char(32) DEFAULT NULL,
  `tool_call_id` char(32) DEFAULT NULL,
  `file_path` varchar(500) NOT NULL,
  `base_content_hash` char(64) NOT NULL DEFAULT '',
  `old_text` text NOT NULL,
  `new_text` text NOT NULL,
  `reason` text DEFAULT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `applied_at` datetime(6) DEFAULT NULL,
  `rejected_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_proposal_project_status` (`project_id`,`status`),
  KEY `ix_proposal_run` (`run_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_proposal_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_proposal_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
