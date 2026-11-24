-- norm_packs 表结构（MySQL/TiDB 方言，真相源）
-- 学术规范包：来源与版本的单位。内置包全局只读；用户包按 owner_id 隔离。
-- 列名用 pack_key 而非 key：key 是 MySQL 保留字。

CREATE TABLE `norm_packs` (
  `id` char(32) NOT NULL,
  `pack_key` varchar(80) NOT NULL,
  `name` varchar(120) NOT NULL,
  `authority` varchar(20) NOT NULL DEFAULT 'general',
  `edition` varchar(40) NOT NULL DEFAULT '',
  `source_url` varchar(500) DEFAULT NULL,
  `source_note` text,
  `scenario` varchar(32) NOT NULL DEFAULT 'all',
  `is_builtin` tinyint(1) NOT NULL DEFAULT '0',
  `owner_id` char(32) DEFAULT NULL,
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  `deleted_seq` bigint NOT NULL DEFAULT '0',
  UNIQUE KEY `uq_norm_pack_key` (`pack_key`,`deleted_seq`),
  KEY `ix_norm_packs_owner_id` (`owner_id`),
  KEY `ix_norm_packs_deleted_at` (`deleted_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_norm_pack_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
