CREATE TABLE norm_packs (
    id uuid PRIMARY KEY,
    pack_key varchar(80) NOT NULL,
    name varchar(120) NOT NULL,
    authority varchar(20) NOT NULL DEFAULT 'general',
    edition varchar(40) NOT NULL DEFAULT '',
    source_url varchar(500),
    source_note text,
    scenario varchar(32) NOT NULL DEFAULT 'all',
    is_builtin boolean NOT NULL DEFAULT false,
    owner_id uuid REFERENCES users (id) ON DELETE CASCADE,
    state varchar(20) NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_seq bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX uq_norm_pack_key ON norm_packs (pack_key, deleted_seq);
CREATE INDEX ix_norm_packs_owner_id ON norm_packs (owner_id);
CREATE INDEX ix_norm_packs_deleted_at ON norm_packs (deleted_at);

CREATE TABLE norm_entries (
    id uuid PRIMARY KEY,
    pack_id uuid NOT NULL REFERENCES norm_packs (id) ON DELETE CASCADE,
    code varchar(60) NOT NULL,
    category varchar(32) NOT NULL,
    rule_kind varchar(16) NOT NULL DEFAULT 'general',
    binding varchar(16) NOT NULL DEFAULT 'should',
    scenario varchar(32) NOT NULL DEFAULT 'all',
    current_version integer NOT NULL DEFAULT 1 CHECK (current_version > 0),
    sort_order integer NOT NULL DEFAULT 0,
    state varchar(20) NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_seq bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX uq_norm_entry_code ON norm_entries (pack_id, code, deleted_seq);
CREATE INDEX ix_norm_entries_pack ON norm_entries (pack_id);
CREATE INDEX ix_norm_entries_category ON norm_entries (category);
CREATE INDEX ix_norm_entries_deleted_at ON norm_entries (deleted_at);

CREATE TABLE norm_versions (
    id uuid PRIMARY KEY,
    entry_id uuid NOT NULL REFERENCES norm_entries (id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    title varchar(200) NOT NULL,
    rule_text text NOT NULL,
    rationale text NOT NULL DEFAULT '',
    examples jsonb NOT NULL,
    applicable_sections jsonb NOT NULL,
    source_ref varchar(300) NOT NULL,
    source_quote text NOT NULL DEFAULT '',
    source_url varchar(500) NOT NULL DEFAULT '',
    content_hash varchar(64) NOT NULL,
    origin varchar(16) NOT NULL DEFAULT 'user',
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_norm_version ON norm_versions (entry_id, version);
CREATE INDEX ix_norm_versions_entry ON norm_versions (entry_id);

CREATE TABLE norm_entry_tags (
    entry_id uuid NOT NULL REFERENCES norm_entries (id) ON DELETE CASCADE,
    tag varchar(32) NOT NULL,
    PRIMARY KEY (entry_id, tag)
);
CREATE INDEX ix_norm_entry_tags_tag ON norm_entry_tags (tag);

CREATE TABLE project_norm_adoptions (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    entry_id uuid NOT NULL REFERENCES norm_entries (id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    state varchar(16) NOT NULL DEFAULT 'enabled',
    override_text text,
    override_updated_at timestamptz,
    adopted_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_seq bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX uq_project_norm_adoption ON project_norm_adoptions (project_id, entry_id, deleted_seq);
CREATE INDEX ix_project_norm_adoptions_project ON project_norm_adoptions (project_id);
CREATE INDEX ix_project_norm_adoptions_entry ON project_norm_adoptions (entry_id);
CREATE INDEX ix_project_norm_adoptions_deleted_at ON project_norm_adoptions (deleted_at);
