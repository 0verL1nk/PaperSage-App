-- agent_wiki_sources 表结构（MySQL/TiDB 方言，真相源）
-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，
-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。
--
-- wiki 是派生笔记，不是新的原始证据。每页的来源要能回溯到
-- 来源 ID / 版本 / 片段，否则「写进 wiki 就当事实」无法核验
-- （docs/project/12-ai-writing-design.md §6.3）。

CREATE TABLE `agent_wiki_sources` (
  `id` char(32) NOT NULL,
  `wiki_page_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL,
  `source_id` char(32) NOT NULL,
  `source_version` char(64) DEFAULT NULL,
  `chunk_id` varchar(80) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_wiki_source_page` (`wiki_page_id`),
  KEY `ix_wiki_source_source` (`source_type`,`source_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_wiki_source_page` FOREIGN KEY (`wiki_page_id`) REFERENCES `agent_wiki_pages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
