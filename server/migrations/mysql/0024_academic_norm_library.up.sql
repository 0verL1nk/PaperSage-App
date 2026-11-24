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
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_norm_pack_key` (`pack_key`,`deleted_seq`),
  KEY `ix_norm_packs_owner_id` (`owner_id`),
  KEY `ix_norm_packs_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_norm_pack_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

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
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_norm_entry_code` (`pack_id`,`code`,`deleted_seq`),
  KEY `ix_norm_entries_pack` (`pack_id`),
  KEY `ix_norm_entries_category` (`category`),
  KEY `ix_norm_entries_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_norm_entry_pack` FOREIGN KEY (`pack_id`) REFERENCES `norm_packs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

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
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_norm_version` (`entry_id`,`version`),
  KEY `ix_norm_versions_entry` (`entry_id`),
  CONSTRAINT `fk_norm_version_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `norm_entry_tags` (
  `entry_id` char(32) NOT NULL,
  `tag` varchar(32) NOT NULL,
  PRIMARY KEY (`entry_id`,`tag`),
  KEY `ix_norm_entry_tags_tag` (`tag`),
  CONSTRAINT `fk_norm_entry_tag_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `project_norm_adoptions` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `entry_id` char(32) NOT NULL,
  `version` int NOT NULL,
  `state` varchar(16) NOT NULL DEFAULT 'enabled',
  `override_text` text,
  `override_updated_at` datetime(6) DEFAULT NULL,
  `adopted_by` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  `deleted_seq` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_project_norm_adoption` (`project_id`,`entry_id`,`deleted_seq`),
  KEY `ix_project_norm_adoptions_project` (`project_id`),
  KEY `ix_project_norm_adoptions_entry` (`entry_id`),
  KEY `ix_project_norm_adoptions_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_norm_adoption_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_norm_adoption_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
