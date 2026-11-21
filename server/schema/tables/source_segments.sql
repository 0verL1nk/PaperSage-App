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
