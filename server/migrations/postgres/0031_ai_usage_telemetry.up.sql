-- AI 调用计量（一次调用一行，保留期硬删）与 Run 的功能入口快照。
-- 删除 agent_runs 的 input_tokens/output_tokens/latency_ms：全仓从未写入，
-- 保留只会与 ai_model_calls 形成两套账。

CREATE TABLE ai_model_calls (
    id                uuid        PRIMARY KEY,
    user_id           uuid        REFERENCES users (id) ON DELETE SET NULL,
    project_id        uuid        REFERENCES projects (id) ON DELETE SET NULL,
    run_id            uuid,
    feature_kind      varchar(32) NOT NULL,
    provider          varchar(64) NOT NULL,
    provider_source   varchar(20) NOT NULL,
    model             varchar(128) NOT NULL,
    is_stream         boolean     NOT NULL DEFAULT false,
    status            varchar(16) NOT NULL,
    error_code        varchar(32),
    http_status       smallint,
    prompt_tokens     int,
    completion_tokens int,
    reasoning_tokens  int,
    latency_ms        int         NOT NULL,
    first_token_ms    int,
    created_day       char(10)    NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_ai_calls_created ON ai_model_calls (created_at);
CREATE INDEX ix_ai_calls_user_created ON ai_model_calls (user_id, created_at);
CREATE INDEX ix_ai_calls_project_created ON ai_model_calls (project_id, created_at);
CREATE INDEX ix_ai_calls_day_model ON ai_model_calls (created_day, model);
CREATE INDEX ix_ai_calls_feature_day ON ai_model_calls (feature_kind, created_day);
CREATE INDEX ix_ai_calls_run ON ai_model_calls (run_id);

ALTER TABLE agent_runs ADD COLUMN feature_kind varchar(32) NOT NULL DEFAULT 'chat';
ALTER TABLE agent_runs DROP COLUMN IF EXISTS input_tokens;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS output_tokens;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS latency_ms;
