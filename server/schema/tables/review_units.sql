-- review_units 表结构（MySQL/TiDB 方言，真相源）
-- 单元 = (章节, 维度) 或跨章维度；findings 经服务端校验后落这里，再聚合为报告。
CREATE TABLE `review_units` (
  `id` char(32) NOT NULL,
  `review_run_id` char(32) NOT NULL,
  `seq` int NOT NULL,
  `kind` varchar(16) NOT NULL,
  `label` varchar(200) NOT NULL,
  `file_paths_json` json NOT NULL,
  `section_ids_json` json NOT NULL,
  `dimensions_json` json NOT NULL,
  `context_json` json NOT NULL,
  `input_fingerprint` char(64) NOT NULL,
  `status` varchar(16) NOT NULL,
  `agent_run_id` char(32) DEFAULT NULL,
  `reused_from_unit_id` char(32) DEFAULT NULL,
  `findings_json` json NOT NULL,
  `dimension_results_json` json NOT NULL,
  `sources_read_json` json NOT NULL,
  `error_text` varchar(512) NOT NULL DEFAULT '',
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  UNIQUE KEY `uq_review_units_seq` (`review_run_id`,`seq`),
  KEY `ix_review_units_fingerprint` (`input_fingerprint`,`status`),
  CONSTRAINT `fk_review_units_run` FOREIGN KEY (`review_run_id`) REFERENCES `review_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
