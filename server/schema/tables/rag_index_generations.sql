-- rag_index_generations 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- 记录每个项目的检索索引代次及其状态、嵌入配置。
-- 来源：docs/project/12-ai-writing-design.md §6.3
--   「建立新索引代次—校验—切换活动代次」；新旧向量不得混用，
--   更换嵌入模型必须重建（因此代次上记录模型标识与维度）。

CREATE TABLE `rag_index_generations` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `generation` bigint NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'building',
  `embedding_model` varchar(128) NOT NULL DEFAULT '',
  `embedding_dim` int NOT NULL DEFAULT '0',
  `chunk_count` int NOT NULL DEFAULT '0',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `activated_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_rag_generation` (`project_id`,`generation`),
  KEY `ix_rag_generation_status` (`project_id`,`status`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rg_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
