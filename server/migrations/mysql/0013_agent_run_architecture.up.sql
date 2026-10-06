-- 0013：Agent 运行时（openspec/changes/agent-run-architecture）
-- 见 postgres/0013_agent_run_architecture.up.sql 的完整说明。
-- 本树不写 IF EXISTS 守卫（golang-migrate 每版本只跑一次）。

-- ── 1. 新表 ─────────────────────────────────────────────────────────
CREATE TABLE agent_turns (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `user_message_id` char(32) DEFAULT NULL,
  `root_run_id` char(32) DEFAULT NULL,
  `client_request_id` varchar(64) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_turn_client_request` (`conversation_id`,`client_request_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_turn_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_message_parts (
  `id` char(32) NOT NULL,
  `message_id` char(32) NOT NULL,
  `ordinal` int NOT NULL DEFAULT '0',
  `type` varchar(20) NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'completed',
  `text_content` text DEFAULT NULL,
  `content_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `completed_at` datetime(6) DEFAULT NULL,
  UNIQUE KEY `uq_part_ordinal` (`message_id`,`ordinal`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_part_message` FOREIGN KEY (`message_id`) REFERENCES `agent_messages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_runs (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `turn_id` char(32) DEFAULT NULL,
  `parent_run_id` char(32) DEFAULT NULL,
  `agent_key` varchar(64) NOT NULL DEFAULT 'main',
  `status` varchar(20) NOT NULL DEFAULT 'running',
  `output_message_id` char(32) DEFAULT NULL,
  `model_provider` varchar(64) NOT NULL DEFAULT '',
  `model_name` varchar(128) NOT NULL DEFAULT '',
  `prompt_version` varchar(32) NOT NULL DEFAULT '',
  `temperature` decimal(4,2) DEFAULT NULL,
  `input_tokens` int NOT NULL DEFAULT '0',
  `output_tokens` int NOT NULL DEFAULT '0',
  `latency_ms` bigint NOT NULL DEFAULT '0',
  `last_sequence` bigint NOT NULL DEFAULT '0',
  `error_json` json DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  KEY `ix_runs_conversation_started` (`conversation_id`,`started_at`),
  KEY `ix_runs_turn` (`turn_id`),
  KEY `ix_runs_parent` (`parent_run_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_run_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `agent_conversations` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_run_parent` FOREIGN KEY (`parent_run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_tool_calls (
  `id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `seq` int NOT NULL DEFAULT '0',
  `tool_name` varchar(64) NOT NULL,
  `state` varchar(24) NOT NULL DEFAULT 'ready',
  `risk_level` varchar(12) NOT NULL DEFAULT 'read',
  `requires_approval` tinyint(1) NOT NULL DEFAULT '0',
  `input_json` json DEFAULT NULL,
  `output_json` json DEFAULT NULL,
  `artifact_id` char(32) DEFAULT NULL,
  `approval_id` varchar(64) DEFAULT NULL,
  `arguments_hash` char(64) DEFAULT NULL,
  `idempotency_key` varchar(128) DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `error_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_tool_run_seq` (`run_id`,`seq`),
  UNIQUE KEY `uq_tool_idempotency` (`idempotency_key`),
  KEY `ix_tool_state` (`state`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_tool_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_events (
  `id` char(32) NOT NULL,
  `conversation_id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `sequence` bigint NOT NULL,
  `event_type` varchar(48) NOT NULL,
  `entity_type` varchar(32) DEFAULT NULL,
  `entity_id` char(32) DEFAULT NULL,
  `payload_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY `uq_event_run_seq` (`run_id`,`sequence`),
  KEY `ix_event_conversation` (`conversation_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_event_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_artifacts (
  `id` char(32) NOT NULL,
  `run_id` char(32) DEFAULT NULL,
  `project_id` char(32) NOT NULL,
  `kind` varchar(32) NOT NULL DEFAULT 'text',
  `s3_key` varchar(500) NOT NULL,
  `mime` varchar(128) NOT NULL DEFAULT 'text/plain',
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `summary_json` json DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_artifact_run` (`run_id`),
  KEY `ix_artifact_project` (`project_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_artifact_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_artifact_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_change_proposals (
  `id` char(32) NOT NULL,
  `project_id` char(32) NOT NULL,
  `run_id` char(32) DEFAULT NULL,
  `tool_call_id` char(32) DEFAULT NULL,
  `file_path` varchar(500) NOT NULL,
  `base_content_hash` char(64) NOT NULL DEFAULT '',
  `old_text` text NOT NULL,
  `new_text` text NOT NULL,
  `reason` text DEFAULT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `applied_at` datetime(6) DEFAULT NULL,
  `rejected_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_proposal_project_status` (`project_id`,`status`),
  KEY `ix_proposal_run` (`run_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_proposal_project` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_proposal_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_evidence (
  `id` char(32) NOT NULL,
  `run_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL,
  `source_id` char(32) NOT NULL,
  `source_version` char(64) DEFAULT NULL,
  `chunk_id` varchar(80) DEFAULT NULL,
  `locator` varchar(255) DEFAULT NULL,
  `quote` text DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_evidence_run` (`run_id`),
  KEY `ix_evidence_source` (`source_type`,`source_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_evidence_run` FOREIGN KEY (`run_id`) REFERENCES `agent_runs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_wiki_sources (
  `id` char(32) NOT NULL,
  `wiki_page_id` char(32) NOT NULL,
  `source_type` varchar(20) NOT NULL,
  `source_id` char(32) NOT NULL,
  `source_version` char(64) DEFAULT NULL,
  `chunk_id` varchar(80) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY `ix_wiki_source_page` (`wiki_page_id`),
  KEY `ix_wiki_source_source` (`source_type`,`source_id`),
  PRIMARY KEY (`id`) /*T![clustered_index] CLUSTERED */,
  CONSTRAINT `fk_wiki_source_page` FOREIGN KEY (`wiki_page_id`) REFERENCES `agent_wiki_pages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ── 2. 改造既有表 ───────────────────────────────────────────────────
ALTER TABLE agent_conversations ADD COLUMN version bigint NOT NULL DEFAULT 0;
ALTER TABLE agent_wiki_pages ADD COLUMN source_run_id char(32) DEFAULT NULL;
ALTER TABLE agent_wiki_pages ADD COLUMN stale_at datetime(6) DEFAULT NULL;
ALTER TABLE agent_messages ADD COLUMN turn_id char(32) DEFAULT NULL;
ALTER TABLE agent_messages ADD COLUMN run_id char(32) DEFAULT NULL;
ALTER TABLE agent_messages ADD COLUMN status varchar(20) NOT NULL DEFAULT 'completed';
ALTER TABLE agent_messages ADD COLUMN completed_at datetime(6) DEFAULT NULL;
CREATE INDEX ix_agent_messages_run ON agent_messages (run_id);

-- ── 3. 数据迁移：content / tool_events → Part ───────────────────────
INSERT INTO agent_message_parts (id, message_id, ordinal, type, status, text_content, content_json, created_at, completed_at)
SELECT MD5(CONCAT(m.id, ':0')), m.id, 0, 'text', 'completed', m.content, NULL, m.created_at, m.created_at
  FROM agent_messages m
 WHERE m.content IS NOT NULL AND m.content <> '';

-- 整段保留为一条 tool part（不逐项展开：需 JSON_TABLE 等方言特有函数，
-- 且这是演示数据，保留原文即可满足审计，避免两方言迁移分叉）。
INSERT INTO agent_message_parts (id, message_id, ordinal, type, status, text_content, content_json, created_at, completed_at)
SELECT MD5(CONCAT(m.id, ':1')), m.id, 1, 'tool', 'completed', NULL, m.tool_events, m.created_at, m.created_at
  FROM agent_messages m
 WHERE m.tool_events IS NOT NULL;

ALTER TABLE agent_messages DROP COLUMN content;
ALTER TABLE agent_messages DROP COLUMN tool_events;
