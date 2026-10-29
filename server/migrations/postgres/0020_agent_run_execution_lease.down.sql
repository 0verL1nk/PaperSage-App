DROP INDEX IF EXISTS ix_runs_lease_expiry;

ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS cancel_requested_at,
    DROP COLUMN IF EXISTS execution_lease_until,
    DROP COLUMN IF EXISTS execution_lease_token;
