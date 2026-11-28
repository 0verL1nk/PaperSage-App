-- 回滚：恢复 agent_runs 的三列（历史 token 数据本来就不存在，按默认值 0 恢复），
-- 删除 Run 的功能入口快照与调用计量表。

ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS input_tokens int NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS output_tokens int NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS latency_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS feature_kind;

DROP TABLE IF EXISTS ai_model_calls;
