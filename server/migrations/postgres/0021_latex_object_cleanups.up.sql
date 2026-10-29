CREATE TABLE IF NOT EXISTS latex_object_cleanups (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    object_key varchar(500) NOT NULL UNIQUE,
    attempts bigint NOT NULL DEFAULT 0,
    last_error varchar(32) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_latex_object_cleanup_project_created
    ON latex_object_cleanups (project_id, created_at);
