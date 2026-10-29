ALTER TABLE agent_runs
    ADD COLUMN execution_lease_token varchar(64),
    ADD COLUMN execution_lease_until timestamptz,
    ADD COLUMN cancel_requested_at timestamptz;

CREATE INDEX IF NOT EXISTS ix_runs_lease_expiry
    ON agent_runs (status, execution_lease_until);
