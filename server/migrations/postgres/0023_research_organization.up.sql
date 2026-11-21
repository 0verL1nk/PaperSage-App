CREATE TABLE research_artifacts (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind varchar(32) NOT NULL,
    state varchar(20) NOT NULL DEFAULT 'active',
    current_revision integer NOT NULL DEFAULT 0 CHECK (current_revision >= 0),
    adopted_revision integer,
    client_request_id varchar(64),
    created_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_seq bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX uq_research_artifact_request ON research_artifacts (project_id, client_request_id, deleted_seq);
CREATE INDEX ix_research_artifacts_project_kind ON research_artifacts (project_id, kind, deleted_at);

CREATE TABLE research_artifact_revisions (
    id uuid PRIMARY KEY,
    artifact_id uuid NOT NULL REFERENCES research_artifacts (id) ON DELETE CASCADE,
    revision integer NOT NULL CHECK (revision > 0),
    payload_json jsonb NOT NULL,
    origin varchar(16) NOT NULL DEFAULT 'user',
    status varchar(20) NOT NULL DEFAULT 'candidate',
    run_id uuid REFERENCES agent_runs (id) ON DELETE SET NULL,
    created_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_artifact_revision UNIQUE (artifact_id, revision)
);
CREATE INDEX ix_artifact_revision_status ON research_artifact_revisions (artifact_id, status);

CREATE TABLE research_artifact_dependencies (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    artifact_id uuid NOT NULL REFERENCES research_artifacts (id) ON DELETE CASCADE,
    revision integer NOT NULL,
    kind varchar(24) NOT NULL,
    ref_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_artifact_dependency UNIQUE (artifact_id, revision, kind, ref_id)
);
CREATE INDEX ix_artifact_dependency_ref ON research_artifact_dependencies (project_id, kind, ref_id);
