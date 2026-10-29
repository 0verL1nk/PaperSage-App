ALTER TABLE agent_runs
  DROP INDEX ix_runs_lease_expiry,
  DROP COLUMN cancel_requested_at,
  DROP COLUMN execution_lease_until,
  DROP COLUMN execution_lease_token;
