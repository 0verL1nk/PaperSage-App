-- 浏览器 WASM 编译失败诊断（运维用，admin 可见）。
-- 与 compile_jobs（用户可见的编译历史）职责分离：本表只存脱敏诊断，
-- 严禁论文正文；靠保留期硬删，不做软删。
CREATE TABLE compile_error_reports (
    id uuid PRIMARY KEY,
    user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    project_id uuid REFERENCES projects (id) ON DELETE SET NULL,
    -- 任务可能随项目级联删除；诊断记录要留存，故不建外键。
    compile_job_id uuid,
    is_guest boolean NOT NULL DEFAULT false,
    client_kind varchar(16) NOT NULL DEFAULT 'web',
    engine varchar(20) NOT NULL,
    engine_version varchar(64),
    resource_profile varchar(64),
    failure_kind varchar(16) NOT NULL,
    failure_stage varchar(24) NOT NULL,
    duration_ms integer NOT NULL DEFAULT 0,
    http_status smallint,
    error_message varchar(512) NOT NULL,
    log_excerpt text,
    failed_resources jsonb,
    client_env jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_cer_created ON compile_error_reports (created_at);
CREATE INDEX ix_cer_user ON compile_error_reports (user_id, created_at);
CREATE INDEX ix_cer_project ON compile_error_reports (project_id, created_at);
CREATE INDEX ix_cer_kind ON compile_error_reports (failure_kind, created_at);
CREATE INDEX ix_cer_engine ON compile_error_reports (engine, created_at);
