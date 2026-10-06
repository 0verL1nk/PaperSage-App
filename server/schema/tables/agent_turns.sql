-- agent_turns 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 一次用户提交 = 一个 Turn（对齐参考架构 threads/turns）。
-- client_request_id 是幂等键：用户双击发送或网络重试不得产生第二个 Turn，
-- 也不得触发第二次模型调用。

CREATE TABLE `agent_turns` (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `user_message_id` char(32) DEFAULT NULL,
  `root_run_id` char(32) DEFAULT NULL,
  `client_request_id` varchar(64) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_turn_client_request` (`conversation_id`,`client_request_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_turn_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
