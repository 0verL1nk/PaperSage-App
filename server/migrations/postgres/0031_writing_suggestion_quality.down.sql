DROP INDEX IF EXISTS ix_proposal_dedupe;

ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS reverted_at;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS drop_reason;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS reject_note;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS reject_reason;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS origin;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS section_path;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS paragraph_hash;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS anchor_hash;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS confidence_reason;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS confidence;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS suggestion_kind;
ALTER TABLE agent_change_proposals DROP COLUMN IF EXISTS basis;

ALTER TABLE agent_runs DROP COLUMN IF EXISTS focus_json;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS trigger;
