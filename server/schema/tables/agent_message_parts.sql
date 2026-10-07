-- agent_message_parts 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 消息由 Part 组成：文本、工具调用、子 agent 活动、审批、引用、大结果、错误
-- 各自成一条，而不是把一切塞进一个 content 字段（这是参考架构点名的反模式）。
-- ordinal 决定消息内顺序，前端 Event Reducer 据此幂等归并。

CREATE TABLE `agent_message_parts` (
  `id` char(32) NOT NULL,
  `message_id` char(32) NOT NULL,
  `ordinal` int NOT NULL DEFAULT '0',
  `type` varchar(20) NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'completed',
  `text_content` text DEFAULT NULL,
  `content_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `completed_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_part_ordinal` (`message_id`,`ordinal`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_part_message` FOREIGN KEY (`message_id`) REFERENCES `agent_messages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
