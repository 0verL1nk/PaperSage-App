-- Completed extraction content is immutable; leases only govern processing.
CREATE TABLE `source_extractions` (
  `id` char(32) NOT NULL,
  `source_version_id` char(32) NOT NULL,
  `extractor_version` varchar(64) NOT NULL,
  `state` varchar(20) NOT NULL DEFAULT 'pending',
  `text_s3_key` varchar(500) DEFAULT NULL,
  `text_sha256` varchar(64) DEFAULT NULL,
  `text_bytes` bigint NOT NULL DEFAULT 0,
  `lease_token` varchar(64) DEFAULT NULL,
  `lease_until` datetime(6) DEFAULT NULL,
  `attempt_count` bigint NOT NULL DEFAULT 0,
  `error_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_source_extractions_version` (`source_version_id`,`extractor_version`),
  CONSTRAINT `fk_source_extractions_version` FOREIGN KEY (`source_version_id`) REFERENCES `source_versions` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
