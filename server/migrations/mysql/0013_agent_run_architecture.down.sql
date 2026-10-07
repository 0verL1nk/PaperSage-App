-- 0013 回滚：尽力还原 Part → content / tool_events，但 Part 的类型与顺序
-- 在旧 schema 下无法完整表达，非 text/tool 的 part 会丢失。

ALTER TABLE agent_messages ADD COLUMN content text NOT NULL;
ALTER TABLE agent_messages ADD COLUMN tool_events json DEFAULT NULL;

UPDATE agent_messages m
  JOIN agent_message_parts p ON p.message_id = m.id AND p.type = 'text' AND p.ordinal = 0
   SET m.content = p.text_content;

UPDATE agent_messages m
  JOIN agent_message_parts p ON p.message_id = m.id AND p.type = 'tool'
   SET m.tool_events = p.content_json;

DROP INDEX ix_agent_messages_run ON agent_messages;
ALTER TABLE agent_messages DROP COLUMN run_id;
ALTER TABLE agent_messages DROP COLUMN turn_id;
ALTER TABLE agent_messages DROP COLUMN status;
ALTER TABLE agent_messages DROP COLUMN completed_at;
ALTER TABLE agent_conversations DROP COLUMN version;
ALTER TABLE agent_wiki_pages DROP COLUMN stale_at;
ALTER TABLE agent_wiki_pages DROP COLUMN source_run_id;

DROP TABLE IF EXISTS agent_wiki_sources;
DROP TABLE IF EXISTS agent_evidence;
DROP TABLE IF EXISTS agent_change_proposals;
DROP TABLE IF EXISTS agent_artifacts;
DROP TABLE IF EXISTS agent_events;
DROP TABLE IF EXISTS agent_tool_calls;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_message_parts;
DROP TABLE IF EXISTS agent_turns;
