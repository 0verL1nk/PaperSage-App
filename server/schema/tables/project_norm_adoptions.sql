-- project_norm_adoptions 表结构（MySQL/TiDB 方言，真相源）
-- 项目对规范条目的采用：version 锁定采用时的版本，管理员改条目不会静默改变
-- 已有项目的约束；升级必须是用户显式动作。

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
  UNIQUE KEY `uq_project_norm_adoption` (`project_id`,`entry_id`,`deleted_seq`),
  KEY `ix_project_norm_adoptions_project` (`project_id`),
  KEY `ix_project_norm_adoptions_entry` (`entry_id`),
  KEY `ix_project_norm_adoptions_deleted_at` (`deleted_at`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_norm_adoption_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_norm_adoption_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
