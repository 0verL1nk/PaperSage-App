-- Publish a version and advance current_revision in the same transaction.
CREATE TABLE project_sources (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    kind varchar(32) NOT NULL,
    current_revision bigint NOT NULL DEFAULT 0 CHECK (current_revision >= 0),
    state varchar(20) NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_project_sources_id_project UNIQUE (id, project_id)
);
CREATE INDEX ix_project_sources_project ON project_sources(project_id);

CREATE TABLE source_versions (
    id uuid PRIMARY KEY,
    source_id uuid NOT NULL REFERENCES project_sources(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision > 0),
    s3_key varchar(500) NOT NULL,
    content_sha256 varchar(64) NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    restored_from_id uuid REFERENCES source_versions(id) ON DELETE RESTRICT,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_source_versions_revision UNIQUE (source_id, revision)
);

CREATE TABLE source_extractions (
    id uuid PRIMARY KEY,
    source_version_id uuid NOT NULL REFERENCES source_versions(id) ON DELETE RESTRICT,
    extractor_version varchar(64) NOT NULL,
    state varchar(20) NOT NULL DEFAULT 'pending',
    text_s3_key varchar(500),
    text_sha256 varchar(64),
    text_bytes bigint NOT NULL DEFAULT 0,
    lease_token varchar(64),
    lease_until timestamptz,
    attempt_count bigint NOT NULL DEFAULT 0,
    error_json jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_source_extractions_version UNIQUE (source_version_id, extractor_version)
);

CREATE TABLE source_segments (
    id uuid PRIMARY KEY,
    extraction_id uuid NOT NULL REFERENCES source_extractions(id) ON DELETE RESTRICT,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    start_byte bigint NOT NULL CHECK (start_byte >= 0),
    end_byte bigint NOT NULL CHECK (end_byte >= start_byte),
    locator_kind varchar(32) NOT NULL,
    physical_page integer CHECK (physical_page IS NULL OR physical_page > 0),
    paragraph_id varchar(128),
    CONSTRAINT uq_source_segments_ordinal UNIQUE (extraction_id, ordinal)
);

ALTER TABLE documents ADD COLUMN source_id uuid;
ALTER TABLE documents ADD CONSTRAINT fk_documents_source FOREIGN KEY (source_id, project_id) REFERENCES project_sources(id, project_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX uq_documents_source ON documents(source_id);
ALTER TABLE latex_files ADD COLUMN source_id uuid;
ALTER TABLE latex_files ADD CONSTRAINT fk_latex_files_source FOREIGN KEY (source_id, project_id) REFERENCES project_sources(id, project_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX uq_latex_files_source ON latex_files(source_id);
