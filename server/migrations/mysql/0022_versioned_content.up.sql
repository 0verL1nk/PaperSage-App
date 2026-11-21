-- The matching version and revision projection must be published atomically.
CREATE TABLE `project_sources` (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL,
  `current_revision` bigint NOT NULL DEFAULT 0,
  `state` varchar(20) NOT NULL DEFAULT 'active',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_project_sources_id_project` (`id`,`project_id`),
  KEY `ix_project_sources_project` (`project_id`),
  CONSTRAINT `ck_project_sources_revision` CHECK (`current_revision` >= 0),
  CONSTRAINT `fk_project_sources_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

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

-- Ranges address UTF-8 bytes in the associated extraction.
CREATE TABLE `source_segments` (
  `id` char(32) NOT NULL,
  `extraction_id` char(32) NOT NULL,
  `ordinal` int NOT NULL,
  `start_byte` bigint NOT NULL,
  `end_byte` bigint NOT NULL,
  `locator_kind` varchar(32) NOT NULL,
  `physical_page` int DEFAULT NULL,
  `paragraph_id` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_source_segments_ordinal` (`extraction_id`,`ordinal`),
  CONSTRAINT `ck_source_segments_ordinal` CHECK (`ordinal` >= 0),
  CONSTRAINT `ck_source_segments_range` CHECK (`start_byte` >= 0 AND `end_byte` >= `start_byte`),
  CONSTRAINT `ck_source_segments_page` CHECK (`physical_page` IS NULL OR `physical_page` > 0),
  CONSTRAINT `fk_source_segments_extraction` FOREIGN KEY (`extraction_id`) REFERENCES `source_extractions` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE documents ADD COLUMN source_id char(32) DEFAULT NULL, ADD UNIQUE KEY uq_documents_source (source_id), ADD CONSTRAINT fk_documents_source FOREIGN KEY (source_id, project_id) REFERENCES project_sources(id, project_id) ON DELETE RESTRICT;
ALTER TABLE latex_files ADD COLUMN source_id char(32) DEFAULT NULL, ADD UNIQUE KEY uq_latex_files_source (source_id), ADD CONSTRAINT fk_latex_files_source FOREIGN KEY (source_id, project_id) REFERENCES project_sources(id, project_id) ON DELETE RESTRICT;
