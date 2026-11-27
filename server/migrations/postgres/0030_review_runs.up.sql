-- 跨章节检查任务：一次检查 = 一个 review_run + N 个 review_unit（按 章节×维度 拆分）。
-- 报告本身仍落在 research_artifacts(kind=chapter_review)，本表只记录执行过程与范围。
ALTER TABLE agent_conversations ADD COLUMN kind varchar(20) NOT NULL DEFAULT 'chat';
DROP INDEX IF EXISTS ix_agent_conversations_list;
CREATE INDEX ix_agent_conversations_list ON agent_conversations (project_id, user_id, kind, updated_at);

CREATE TABLE review_runs (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- 检查专用隐藏会话（kind='review'），不出现在聊天列表。
    conversation_id uuid NOT NULL,
    client_request_id varchar(64) NOT NULL,
    status varchar(20) NOT NULL,
    scope_json jsonb NOT NULL,
    incremental boolean NOT NULL DEFAULT false,
    model_provider varchar(64) NOT NULL DEFAULT '',
    model_name varchar(128) NOT NULL DEFAULT '',
    prompt_version varchar(32) NOT NULL DEFAULT '',
    artifact_id uuid,
    artifact_revision integer,
    error_text varchar(512) NOT NULL DEFAULT '',
    cancel_requested_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, client_request_id)
);
CREATE INDEX ix_review_runs_project_created ON review_runs (project_id, created_at DESC);
CREATE INDEX ix_review_runs_status ON review_runs (status);

CREATE TABLE review_units (
    id uuid PRIMARY KEY,
    review_run_id uuid NOT NULL REFERENCES review_runs (id) ON DELETE CASCADE,
    seq integer NOT NULL,
    kind varchar(16) NOT NULL,
    label varchar(200) NOT NULL,
    file_paths_json jsonb NOT NULL,
    section_ids_json jsonb NOT NULL,
    dimensions_json jsonb NOT NULL,
    -- 服务端在创建时锁定的单元上下文；运行期间文件变化不影响本次输入。
    context_json jsonb NOT NULL,
    input_fingerprint char(64) NOT NULL,
    status varchar(16) NOT NULL,
    agent_run_id uuid,
    reused_from_unit_id uuid,
    findings_json jsonb NOT NULL DEFAULT '[]',
    dimension_results_json jsonb NOT NULL DEFAULT '[]',
    sources_read_json jsonb NOT NULL DEFAULT '[]',
    error_text varchar(512) NOT NULL DEFAULT '',
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (review_run_id, seq)
);
CREATE INDEX ix_review_units_fingerprint ON review_units (input_fingerprint, status);
