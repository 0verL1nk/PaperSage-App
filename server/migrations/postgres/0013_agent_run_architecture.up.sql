-- 0013：Agent 运行时（openspec/changes/agent-run-architecture）
--
-- 1) 新增 9 张表：turns / message_parts / runs / tool_calls / events /
--    artifacts / change_proposals / evidence / wiki_sources
-- 2) 改造 3 张表：agent_conversations + version；agent_messages 改由 Part 承载内容；
--    agent_wiki_pages + source_run_id / stale_at
-- 3) 数据迁移：现有消息的 content / tool_events 转成 Part，再删除旧列
--
-- 回滚见 .down.sql（无法还原已删除的 content / tool_events 数据）。

-- ── 1. 新表 ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS agent_turns (
    id                uuid        PRIMARY KEY,
    conversation_id   uuid        NOT NULL REFERENCES agent_conversations (id) ON DELETE CASCADE,
    user_message_id   uuid,
    root_run_id       uuid,
    client_request_id varchar(64),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, client_request_id)
);

CREATE TABLE IF NOT EXISTS agent_message_parts (
    id           uuid        PRIMARY KEY,
    message_id   uuid        NOT NULL REFERENCES agent_messages (id) ON DELETE CASCADE,
    ordinal      int         NOT NULL DEFAULT 0,
    type         varchar(20) NOT NULL,
    status       varchar(20) NOT NULL DEFAULT 'completed',
    text_content text,
    content_json json,
    created_at   timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (message_id, ordinal)
);

CREATE TABLE IF NOT EXISTS agent_runs (
    id                uuid        PRIMARY KEY,
    conversation_id   uuid        NOT NULL REFERENCES agent_conversations (id) ON DELETE CASCADE,
    turn_id           uuid,
    parent_run_id     uuid        REFERENCES agent_runs (id) ON DELETE CASCADE,
    agent_key         varchar(64) NOT NULL DEFAULT 'main',
    status            varchar(20) NOT NULL DEFAULT 'running',
    output_message_id uuid,
    model_provider    varchar(64) NOT NULL DEFAULT '',
    model_name        varchar(128) NOT NULL DEFAULT '',
    prompt_version    varchar(32) NOT NULL DEFAULT '',
    temperature       numeric(4,2),
    input_tokens      int         NOT NULL DEFAULT 0,
    output_tokens     int         NOT NULL DEFAULT 0,
    latency_ms        bigint      NOT NULL DEFAULT 0,
    last_sequence     bigint      NOT NULL DEFAULT 0,
    error_json        json,
    started_at        timestamptz,
    completed_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_runs_conversation_started ON agent_runs (conversation_id, started_at);
CREATE INDEX IF NOT EXISTS ix_runs_turn  ON agent_runs (turn_id);
CREATE INDEX IF NOT EXISTS ix_runs_parent ON agent_runs (parent_run_id);

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id                uuid        PRIMARY KEY,
    run_id            uuid        NOT NULL REFERENCES agent_runs (id) ON DELETE CASCADE,
    seq               int         NOT NULL DEFAULT 0,
    tool_name         varchar(64) NOT NULL,
    state             varchar(24) NOT NULL DEFAULT 'ready',
    risk_level        varchar(12) NOT NULL DEFAULT 'read',
    requires_approval boolean     NOT NULL DEFAULT false,
    input_json        json,
    output_json       json,
    artifact_id       uuid,
    approval_id       varchar(64),
    arguments_hash    char(64),
    idempotency_key   varchar(128),
    started_at        timestamptz,
    completed_at      timestamptz,
    error_json        json,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, seq),
    UNIQUE (idempotency_key)
);
CREATE INDEX IF NOT EXISTS ix_tool_state ON agent_tool_calls (state);

CREATE TABLE IF NOT EXISTS agent_events (
    id              uuid        PRIMARY KEY,
    conversation_id uuid        NOT NULL,
    run_id          uuid        NOT NULL REFERENCES agent_runs (id) ON DELETE CASCADE,
    sequence        bigint      NOT NULL,
    event_type      varchar(48) NOT NULL,
    entity_type     varchar(32),
    entity_id       uuid,
    payload_json    json,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, sequence)
);
CREATE INDEX IF NOT EXISTS ix_event_conversation ON agent_events (conversation_id);

CREATE TABLE IF NOT EXISTS agent_artifacts (
    id           uuid        PRIMARY KEY,
    run_id       uuid        REFERENCES agent_runs (id) ON DELETE SET NULL,
    project_id   uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind         varchar(32) NOT NULL DEFAULT 'text',
    s3_key       varchar(500) NOT NULL,
    mime         varchar(128) NOT NULL DEFAULT 'text/plain',
    size_bytes   bigint      NOT NULL DEFAULT 0,
    summary_json json,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_artifact_run     ON agent_artifacts (run_id);
CREATE INDEX IF NOT EXISTS ix_artifact_project ON agent_artifacts (project_id);

CREATE TABLE IF NOT EXISTS agent_change_proposals (
    id                uuid        PRIMARY KEY,
    project_id        uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    run_id            uuid        REFERENCES agent_runs (id) ON DELETE CASCADE,
    tool_call_id      uuid,
    file_path         varchar(500) NOT NULL,
    base_content_hash char(64)    NOT NULL DEFAULT '',
    old_text          text        NOT NULL,
    new_text          text        NOT NULL,
    reason            text,
    status            varchar(20) NOT NULL DEFAULT 'pending',
    applied_at        timestamptz,
    rejected_at       timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_proposal_project_status ON agent_change_proposals (project_id, status);
CREATE INDEX IF NOT EXISTS ix_proposal_run ON agent_change_proposals (run_id);

CREATE TABLE IF NOT EXISTS agent_evidence (
    id             uuid        PRIMARY KEY,
    run_id         uuid        NOT NULL REFERENCES agent_runs (id) ON DELETE CASCADE,
    source_type    varchar(20) NOT NULL,
    source_id      uuid        NOT NULL,
    source_version char(64),
    chunk_id       varchar(80),
    locator        varchar(255),
    quote          text,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_evidence_run    ON agent_evidence (run_id);
CREATE INDEX IF NOT EXISTS ix_evidence_source ON agent_evidence (source_type, source_id);

CREATE TABLE IF NOT EXISTS agent_wiki_sources (
    id             uuid        PRIMARY KEY,
    wiki_page_id   uuid        NOT NULL REFERENCES agent_wiki_pages (id) ON DELETE CASCADE,
    source_type    varchar(20) NOT NULL,
    source_id      uuid        NOT NULL,
    source_version char(64),
    chunk_id       varchar(80),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_wiki_source_page   ON agent_wiki_sources (wiki_page_id);
CREATE INDEX IF NOT EXISTS ix_wiki_source_source ON agent_wiki_sources (source_type, source_id);

-- ── 2. 改造既有表 ───────────────────────────────────────────────────
ALTER TABLE agent_conversations ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 0;

ALTER TABLE agent_wiki_pages ADD COLUMN IF NOT EXISTS source_run_id uuid;
ALTER TABLE agent_wiki_pages ADD COLUMN IF NOT EXISTS stale_at timestamptz;

ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS turn_id uuid;
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS run_id uuid;
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS status varchar(20) NOT NULL DEFAULT 'completed';
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS completed_at timestamptz;
CREATE INDEX IF NOT EXISTS ix_agent_messages_run ON agent_messages (run_id);

-- ── 3. 数据迁移：content / tool_events → Part ───────────────────────
-- content 逐条转为一条 text part。
INSERT INTO agent_message_parts (id, message_id, ordinal, type, status, text_content, content_json, created_at, completed_at)
SELECT md5(m.id::text || ':0')::uuid, m.id, 0, 'text', 'completed', m.content, NULL, m.created_at, m.created_at
  FROM agent_messages m
 WHERE m.content IS NOT NULL AND m.content <> '';

-- tool_events 是 JSON 数组。这里整段保留为一条 tool part，而不是逐项展开：
-- 展开需要 JSON_TABLE / jsonb_array_elements 等方言特有函数，而这是演示数据，
-- 保留原文即可满足审计，亦避免两方言迁移分叉。
INSERT INTO agent_message_parts (id, message_id, ordinal, type, status, text_content, content_json, created_at, completed_at)
SELECT md5(m.id::text || ':1')::uuid, m.id, 1, 'tool', 'completed', NULL, m.tool_events, m.created_at, m.created_at
  FROM agent_messages m
 WHERE m.tool_events IS NOT NULL;

ALTER TABLE agent_messages DROP COLUMN IF EXISTS content;
ALTER TABLE agent_messages DROP COLUMN IF EXISTS tool_events;
