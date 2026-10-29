CREATE TABLE IF NOT EXISTS project_purge_operations (
    project_id uuid PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    owner_id uuid NOT NULL,
    attempt_count bigint NOT NULL DEFAULT 0,
    documents_offset bigint NOT NULL DEFAULT 0,
    latex_files_offset bigint NOT NULL DEFAULT 0,
    artifacts_offset bigint NOT NULL DEFAULT 0,
    s3_done boolean NOT NULL DEFAULT false,
    compact_prefix_done boolean NOT NULL DEFAULT false,
    dashed_prefix_done boolean NOT NULL DEFAULT false,
    rag_done boolean NOT NULL DEFAULT false,
    lease_token varchar(36),
    lease_until timestamptz,
    last_error varchar(120) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_project_purge_owner ON project_purge_operations (owner_id);
CREATE INDEX IF NOT EXISTS ix_project_purge_lease ON project_purge_operations (lease_until);
