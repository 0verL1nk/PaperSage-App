-- norm_versions 表结构（MySQL/TiDB 方言，真相源）
-- 不可变内容版本：改条目 = 追加一版。AI 证据与检查报告引用 (entry_id, version)
-- 永久可回查；content_hash 供排障比对。

CREATE TABLE `norm_versions` (
  `id` char(32) NOT NULL,
  `entry_id` char(32) NOT NULL,
  `version` int NOT NULL,
  `title` varchar(200) NOT NULL,
  `rule_text` text NOT NULL,
  `rationale` text NOT NULL,
  `examples` json NOT NULL,
  `applicable_sections` json NOT NULL,
  `source_ref` varchar(300) NOT NULL,
  `source_quote` text NOT NULL,
  `source_url` varchar(500) NOT NULL,
  `content_hash` varchar(64) NOT NULL,
  `origin` varchar(16) NOT NULL DEFAULT 'user',
  `created_by` char(32) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_norm_version` (`entry_id`,`version`),
  KEY `ix_norm_versions_entry` (`entry_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_norm_version_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
