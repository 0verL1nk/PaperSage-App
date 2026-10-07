CREATE TABLE `import_receipts` (
  `id` char(32) NOT NULL,
  `owner_id` char(32) NOT NULL,
  `bundle_id` char(32) NOT NULL,
  `resource_type` varchar(24) NOT NULL,
  `local_id` char(32) NOT NULL,
  `fingerprint` char(64) NOT NULL,
  `resource_id` char(32) NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_import_receipt_scope` (`owner_id`, `bundle_id`, `resource_type`, `local_id`),
  KEY `ix_import_receipts_created_at` (`created_at`),
  CONSTRAINT `fk_import_receipts_owner` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
