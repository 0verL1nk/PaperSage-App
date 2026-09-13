-- 0004：AI 助手会话（云端化：DB 持久，跨端可续）
CREATE TABLE IF NOT EXISTS agent_conversations (
    id         uuid PRIMARY KEY,
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      varchar(200) NOT NULL DEFAULT '新对话',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_agent_conversations_project ON agent_conversations (project_id);

CREATE TABLE IF NOT EXISTS agent_messages (
    id              uuid PRIMARY KEY,
    conversation_id uuid        NOT NULL REFERENCES agent_conversations (id) ON DELETE CASCADE,
    role            varchar(20) NOT NULL, -- user / assistant
    content         text        NOT NULL,
    tool_events     json,                -- 工具调用摘要 [{tool,args,result}]
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_agent_messages_conversation ON agent_messages (conversation_id);
