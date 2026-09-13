CREATE TABLE agent_conversations (
    id         char(32)     PRIMARY KEY,
    project_id char(32)     NOT NULL,
    user_id    char(32)     NOT NULL,
    title      varchar(200) NOT NULL DEFAULT '新对话',
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_agent_conversations_project (project_id),
    CONSTRAINT fk_ac_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
    CONSTRAINT fk_ac_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE agent_messages (
    id              char(32)     PRIMARY KEY,
    conversation_id char(32)     NOT NULL,
    role            varchar(20)  NOT NULL,
    content         text         NOT NULL,
    tool_events     json,
    created_at      datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    KEY ix_agent_messages_conversation (conversation_id),
    CONSTRAINT fk_am_conversation FOREIGN KEY (conversation_id) REFERENCES agent_conversations (id) ON DELETE CASCADE
);
