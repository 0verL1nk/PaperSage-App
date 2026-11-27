DROP INDEX `ix_proposal_dedupe` ON `agent_change_proposals`;

ALTER TABLE `agent_change_proposals` DROP COLUMN `reverted_at`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `drop_reason`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `reject_note`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `reject_reason`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `origin`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `section_path`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `paragraph_hash`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `anchor_hash`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `confidence_reason`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `confidence`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `suggestion_kind`;
ALTER TABLE `agent_change_proposals` DROP COLUMN `basis`;

ALTER TABLE `agent_runs` DROP COLUMN `focus_json`;
ALTER TABLE `agent_runs` DROP COLUMN `trigger`;
