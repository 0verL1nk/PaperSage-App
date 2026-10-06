-- 0013 回滚：尽力还原 Part → content / tool_events，但 Part 的类型信息与
-- 顺序在旧 schema 下无法完整表达，非 text/tool 的 part 会丢失。

-- 还原旧列
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS content text NOT NULL DEFAULT '';
ALTER TABLE agent_messages ADD COLUMN IF NOT EXISTS tool_events json;

UPDATE agent_messages m SET content = p.text_content
  FROM agent_message_parts p
 WHERE p.message_id = m.id AND p.type = 'text' AND p.ordinal = 0;

UPDATE agent_messages m SET tool_events = p.content_json
  FROM agent_message_parts p
 WHERE p.message_id = m.id AND p.type = 'tool';

-- 还原改造
DROP INDEX IF EXISTS ix_agent_messages_run;
ALTER TABLE agent_messages DROP COLUMN IF EXISTS run_id;
ALTER TABLE agent_messages DROP COLUMN IF EXISTS turn_id;
ALTER TABLE agent_messages DROP COLUMN IF EXISTS status;
ALTER TABLE agent_messages DROP COLUMN IF EXISTS completed_at;
ALTER TABLE agent_conversations DROP COLUMN IF EXISTS version;
ALTER TABLE agent_wiki_pages DROP COLUMN IF EXISTS stale_at;
ALTER TABLE agent_wiki_pages DROP COLUMN IF EXISTS source_run_id;

-- 删除新表（先删子表）
DROP TABLE IF EXISTS agent_wiki_sources;
DROP TABLE IF EXISTS agent_evidence;
DROP TABLE IF EXISTS agent_change_proposals;
DROP TABLE IF EXISTS agent_artifacts;
DROP TABLE IF EXISTS agent_events;
DROP TABLE IF EXISTS agent_tool_calls;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_message_parts;
DROP TABLE IF EXISTS agent_turns;
