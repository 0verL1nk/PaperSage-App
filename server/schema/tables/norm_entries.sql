-- norm_entries 表结构（MySQL/TiDB 方言，真相源）
-- 条目聚合根：current_version 指向不可变的 norm_versions；内容变更只追加版本。

CREATE TABLE `norm_entries` (
  `id` char(32) NOT NULL,
  `pack_id` char(32) NOT NULL,
  `code` varchar(60) NOT NULL,
  `category` varchar(32) NOT NULL,
  `rule_kind` varchar(16) NOT NULL DEFAULT 'general',
  `binding` varchar(16) NOT NULL DEFAULT 'should',
  `scenario` varchar(32) NOT NULL DEFAULT 'all',
  `current_version` int NOT NULL DEFAULT '1',
  `sort_order` int NOT NULL DEFAULT '0',
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  `deleted_seq` bigint NOT NULL DEFAULT '0',
  UNIQUE KEY `uq_norm_entry_code` (`pack_id`,`code`,`deleted_seq`),
  KEY `ix_norm_entries_pack` (`pack_id`),
  KEY `ix_norm_entries_category` (`category`),
  KEY `ix_norm_entries_deleted_at` (`deleted_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_norm_entry_pack` FOREIGN KEY (`pack_id`) REFERENCES `norm_packs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
