ALTER TABLE agent_runs
  ADD COLUMN execution_lease_token varchar(64) DEFAULT NULL,
  ADD COLUMN execution_lease_until datetime(6) DEFAULT NULL,
  ADD COLUMN cancel_requested_at datetime(6) DEFAULT NULL,
  ADD INDEX ix_runs_lease_expiry (status, execution_lease_until);
