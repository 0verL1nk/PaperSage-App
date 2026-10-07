CREATE TABLE rag_chunks (
    id          varchar(80)  PRIMARY KEY,
    project_id  char(32)     NOT NULL,
    source_type varchar(20)  NOT NULL DEFAULT 'document',
    source_id   char(32)     NOT NULL,
    source_name varchar(255) NOT NULL DEFAULT '',
    chunk_idx   int          NOT NULL DEFAULT 0,
    content     text         NOT NULL,
    created_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    KEY ix_rag_chunks_project (project_id),
    KEY ix_rag_chunks_source (source_type, source_id),
    CONSTRAINT fk_rc_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_wiki_pages (
    id         char(32)     PRIMARY KEY,
    project_id char(32)     NOT NULL,
    title      varchar(200) NOT NULL,
    content    text         NOT NULL,
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_wiki_project (project_id),
    UNIQUE KEY uq_wiki_project_title (project_id, title),
    CONSTRAINT fk_wp_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
