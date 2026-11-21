-- Version content is immutable; the service validates restore provenance.
CREATE TABLE `source_versions` (
  `id` char(32) NOT NULL,
  `source_id` char(32) NOT NULL,
  `revision` bigint NOT NULL,
  `s3_key` varchar(500) NOT NULL,
  `content_sha256` varchar(64) NOT NULL,
  `size_bytes` bigint NOT NULL,
  `restored_from_id` char(32) DEFAULT NULL,
  `created_by` char(32) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_source_versions_revision` (`source_id`,`revision`),
  CONSTRAINT `ck_source_versions_revision` CHECK (`revision` > 0),
  CONSTRAINT `ck_source_versions_size` CHECK (`size_bytes` >= 0),
  CONSTRAINT `fk_source_versions_source` FOREIGN KEY (`source_id`) REFERENCES `project_sources` (`id`) ON DELETE RESTRICT,
  CONSTRAINT `fk_source_versions_restore` FOREIGN KEY (`restored_from_id`) REFERENCES `source_versions` (`id`) ON DELETE RESTRICT,
  CONSTRAINT `fk_source_versions_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
