-- PaperSage 全表结构快照（MySQL/TiDB 方言）
-- 由 cmd/dumpschema 生成：全部迁移（migrations/ 全量）执行后的终态。
-- 真相源是 migrations/；改模型请加迁移并重跑 make schema。

CREATE TABLE `agent_conversations` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `user_id` char(32) NOT NULL,
  `title` varchar(200) NOT NULL DEFAULT '新对话',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_agent_conversations_project` (`project_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_ac_user` (`user_id`),
  CONSTRAINT `fk_ac_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ac_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `agent_messages` (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `role` varchar(20) NOT NULL,
  `content` text NOT NULL,
  `tool_events` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_agent_messages_conversation` (`conversation_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_am_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `agent_wiki_pages` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `title` varchar(200) NOT NULL,
  `content` text NOT NULL,
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_wiki_project` (`project_id`),
  UNIQUE KEY `uq_wiki_project_title` (`project_id`,`title`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_wp_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `compile_jobs` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `user_id` char(32) NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `root_file` varchar(500) NOT NULL DEFAULT 'main.tex',
  `compiler` varchar(20) NOT NULL DEFAULT 'pdflatex',
  `build_id` varchar(100) DEFAULT NULL,
  `error` text DEFAULT NULL,
  `logs` text DEFAULT NULL,
  `sync_state` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_compile_jobs_project_id` (`project_id`),
  KEY `ix_compile_jobs_user_id` (`user_id`),
  KEY `ix_compile_jobs_status` (`status`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_cj_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_cj_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `documents` (
  `id` char(32) NOT NULL,
  `name` varchar(255) NOT NULL,
  `type` varchar(100) NOT NULL,
  `size` bigint NOT NULL DEFAULT '0',
  `status` varchar(20) NOT NULL DEFAULT 'uploaded',
  `s3_key` varchar(500) NOT NULL,
  `remark` text DEFAULT NULL,
  `project_id` char(32) NOT NULL,
  `folder_id` char(32) DEFAULT NULL,
  `owner_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_documents_project` (`project_id`),
  KEY `ix_documents_folder` (`folder_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_doc_owner` (`owner_id`),
  CONSTRAINT `fk_doc_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_doc_folder` FOREIGN KEY (`folder_id`) REFERENCES `folders` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_doc_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `folders` (
  `id` char(32) NOT NULL,
  `name` varchar(100) NOT NULL,
  `parent_id` char(32) DEFAULT NULL,
  `project_id` char(32) NOT NULL,
  `owner_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_folders_parent` (`parent_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  KEY `fk_folder_project` (`project_id`),
  KEY `fk_folder_owner` (`owner_id`),
  CONSTRAINT `fk_folder_parent` FOREIGN KEY (`parent_id`) REFERENCES `folders` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_folder_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_folder_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `latex_files` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `file_path` varchar(500) NOT NULL,
  `content_type` varchar(100) DEFAULT NULL,
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `content_hash` varchar(64) DEFAULT NULL,
  `s3_key` varchar(500) DEFAULT NULL,
  `last_saved_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_project_file_path` (`project_id`,`file_path`),
  KEY `ix_latex_files_project_id` (`project_id`),
  KEY `ix_latex_files_content_hash` (`content_hash`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_lf_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `project_groups` (
  `id` char(32) NOT NULL,
  `name` varchar(50) NOT NULL,
  `owner_id` char(32) NOT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_project_groups_owner_id` (`owner_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_pg_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `projects` (
  `id` char(32) NOT NULL,
  `name` varchar(100) NOT NULL,
  `description` text DEFAULT NULL,
  `owner_id` char(32) NOT NULL,
  `group_id` char(32) DEFAULT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `is_deleted` tinyint(1) NOT NULL DEFAULT '0',
  `deleted_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  KEY `ix_projects_owner_id` (`owner_id`),
  KEY `ix_projects_group_id` (`group_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_proj_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_proj_group` FOREIGN KEY (`group_id`) REFERENCES `project_groups` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `rag_chunks` (
  `id` varchar(80) NOT NULL,
  `project_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL DEFAULT 'document',
  `source_id` char(32) NOT NULL,
  `source_name` varchar(255) NOT NULL DEFAULT '',
  `chunk_idx` int NOT NULL DEFAULT '0',
  `content` text NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `embedding` mediumtext DEFAULT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_rag_chunks_project` (`project_id`),
  KEY `ix_rag_chunks_source` (`source_type`,`source_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rc_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `refresh_tokens` (
  `token_hash` varchar(64) NOT NULL,
  `user_id` char(32) NOT NULL,
  `expires_at` datetime(6) NOT NULL,
  `revoked_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_refresh_tokens_user_id` (`user_id`),
  PRIMARY KEY (`token_hash`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_rt_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `schema_migrations` (
  `version` bigint NOT NULL,
  `dirty` tinyint(1) NOT NULL,
  PRIMARY KEY (`version`) /*T![clustered_index] CLUSTERED */
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `system_settings` (
  `key` varchar(100) NOT NULL,
  `value` json NOT NULL,
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`key`) /*T![clustered_index] CLUSTERED */
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `user_settings` (
  `user_id` char(32) NOT NULL,
  `category` varchar(100) NOT NULL,
  `settings` json NOT NULL,
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `deleted_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`user_id`,`category`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_us_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `users` (
  `id` char(32) NOT NULL,
  `username` varchar(50) NOT NULL,
  `password_hash` varchar(255) NOT NULL,
  `email` varchar(255) DEFAULT NULL,
  `email_verified` tinyint(1) NOT NULL DEFAULT '0',
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `is_superuser` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `is_email_registered` tinyint(1) NOT NULL DEFAULT '0',
  `deleted_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `ix_users_username` (`username`),
  UNIQUE KEY `ix_users_email` (`email`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE `verification_tokens` (
  `token_hash` varchar(64) NOT NULL,
  `user_id` char(32) NOT NULL,
  `expires_at` datetime(6) NOT NULL,
  `used_at` datetime(6) DEFAULT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  KEY `ix_vt_user` (`user_id`),
  PRIMARY KEY (`token_hash`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_vt_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

