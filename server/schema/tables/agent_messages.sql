-- agent_messages 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 消息不再自带 content / tool_events：内容全部由 agent_message_parts 承载。
-- 先前把整段文本与工具事件塞进两列 JSON 是参考架构点名的反模式
-- （无法索引、无法聚合，而验收要求统计工具调用次数与用量）。
-- turn_id / run_id 不加外键：agent_turns 与 agent_runs 反过来引用消息 id，
-- 加外键会形成建表顺序环。

CREATE TABLE `agent_messages` (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `turn_id` char(32) DEFAULT NULL,
  `run_id` char(32) DEFAULT NULL,
  `role` varchar(20) NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'completed',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `completed_at` datetime(6) DEFAULT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  -- 历史消息：WHERE conversation_id ORDER BY created_at
  -- （conversation_id 单列索引是其前缀，不再单列重复建）
  KEY `ix_agent_messages_history` (`conversation_id`,`created_at`),
  KEY `ix_agent_messages_run` (`run_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_am_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
