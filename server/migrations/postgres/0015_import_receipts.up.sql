CREATE TABLE import_receipts (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    bundle_id uuid NOT NULL,
    resource_type varchar(24) NOT NULL,
    local_id uuid NOT NULL,
    fingerprint char(64) NOT NULL,
    resource_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_import_receipt_scope UNIQUE (owner_id, bundle_id, resource_type, local_id)
);
CREATE INDEX ix_import_receipts_created_at ON import_receipts (created_at);
