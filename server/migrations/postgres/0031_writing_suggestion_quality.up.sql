-- 动态写作建议质量契约（HELE-69）：只加列/索引、带默认值，旧数据可继续读写。
-- Run 侧记录触发来源与焦点快照；建议侧记录三源依据、置信度、去重 hash 与审计原因。
ALTER TABLE agent_runs ADD COLUMN trigger varchar(20) NOT NULL DEFAULT 'chat';
ALTER TABLE agent_runs ADD COLUMN focus_json jsonb;

ALTER TABLE agent_change_proposals ADD COLUMN basis jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_change_proposals ADD COLUMN suggestion_kind varchar(32) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN confidence varchar(12) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN confidence_reason text;
ALTER TABLE agent_change_proposals ADD COLUMN anchor_hash char(64) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN paragraph_hash char(64) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN section_path varchar(500) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN origin varchar(20) NOT NULL DEFAULT 'chat';
ALTER TABLE agent_change_proposals ADD COLUMN reject_reason varchar(32) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN reject_note text;
ALTER TABLE agent_change_proposals ADD COLUMN drop_reason varchar(64) NOT NULL DEFAULT '';
ALTER TABLE agent_change_proposals ADD COLUMN reverted_at timestamptz;

CREATE INDEX ix_proposal_dedupe ON agent_change_proposals
  (project_id, file_path, paragraph_hash, suggestion_kind, created_at);
