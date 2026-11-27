DROP TABLE IF EXISTS review_units;
DROP TABLE IF EXISTS review_runs;
DROP INDEX IF EXISTS ix_agent_conversations_list;
CREATE INDEX ix_agent_conversations_list ON agent_conversations (project_id, user_id, updated_at);
ALTER TABLE agent_conversations DROP COLUMN IF EXISTS kind;
