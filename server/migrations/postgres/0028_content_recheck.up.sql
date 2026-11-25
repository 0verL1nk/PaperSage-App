-- 资料变更后的派生内容「待复查」标记。
-- 删除/出新版本时由同一事务确定性写入；是否仍成立由用户逐条确认，不自动改写。
CREATE TABLE content_recheck_marks (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    target_type varchar(24) NOT NULL,
    target_id uuid NOT NULL,
    artifact_id uuid,
    revision integer,
    trigger_kind varchar(24) NOT NULL,
    trigger_id uuid NOT NULL,
    reason varchar(500) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    resolved_by uuid REFERENCES users (id) ON DELETE SET NULL,
    resolution varchar(16),
    resolved_seq bigint NOT NULL DEFAULT 0,
    CONSTRAINT uq_recheck_mark UNIQUE (target_type, target_id, trigger_kind, trigger_id, resolved_seq)
);
CREATE INDEX ix_recheck_project ON content_recheck_marks (project_id, resolved_seq);
CREATE INDEX ix_recheck_artifact ON content_recheck_marks (artifact_id, revision, resolved_seq);

-- wiki 来源回溯写入的唯一键（Run 级近似，重复写入幂等）。
ALTER TABLE agent_wiki_sources
    ADD CONSTRAINT uq_wiki_source_trace
    UNIQUE (wiki_page_id, source_type, source_id, source_version, chunk_id);
