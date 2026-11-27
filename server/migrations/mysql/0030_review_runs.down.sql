DROP TABLE IF EXISTS `review_units`;
DROP TABLE IF EXISTS `review_runs`;
DROP INDEX `ix_agent_conversations_kind` ON `agent_conversations`;
ALTER TABLE `agent_conversations` DROP COLUMN `kind`;
