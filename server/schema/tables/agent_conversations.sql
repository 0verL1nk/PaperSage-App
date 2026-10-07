-- agent_conversations 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。

CREATE TABLE `agent_conversations` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `user_id` char(32) NOT NULL,
  `title` varchar(200) NOT NULL DEFAULT '新对话',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  -- 乐观锁：提交消息时携带 expected_thread_version，不匹配则 409，
  -- 避免两个标签页/设备各写一条而互相覆盖。
  `version` bigint NOT NULL DEFAULT '0',
  -- 会话列表：WHERE project_id AND user_id ORDER BY updated_at DESC
  -- （project_id 单列索引是其前缀，不再单列重复建）
  KEY `ix_agent_conversations_list` (`project_id`,`user_id`,`updated_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_ac_user` (`user_id`),
  CONSTRAINT `fk_ac_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ac_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
