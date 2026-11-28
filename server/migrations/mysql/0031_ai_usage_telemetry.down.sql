-- 回滚（MySQL/TiDB）：恢复 agent_runs 的三列（历史数据不存在，按 0 恢复），
-- 删除 Run 的功能入口快照与调用计量表。

ALTER TABLE `agent_runs` ADD COLUMN `input_tokens` int NOT NULL DEFAULT 0;
ALTER TABLE `agent_runs` ADD COLUMN `output_tokens` int NOT NULL DEFAULT 0;
ALTER TABLE `agent_runs` ADD COLUMN `latency_ms` bigint NOT NULL DEFAULT 0;
ALTER TABLE `agent_runs` DROP COLUMN `feature_kind`;

DROP TABLE IF EXISTS `ai_model_calls`;
