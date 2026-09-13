-- 0001_baseline（MySQL/TiDB 方言）—— 幂等：golang-migrate 每版本只跑一次，
-- 故此处不需要 IF NOT EXISTS（MySQL 8 不支持索引级 IF NOT EXISTS）。
-- 真相来源：migrations/postgres/0001（结构对齐）。
-- 类型映射：uuid→char(32)（store.UUID Valuer 存 32 位 hex），
-- timestamptz→datetime(6)（UTC 由应用层保证），json 同名。

CREATE TABLE users (
    id            char(32)     PRIMARY KEY,
    username      varchar(50)  NOT NULL,
    password_hash varchar(255) NOT NULL,
    email         varchar(255),
    email_verified boolean     NOT NULL DEFAULT false,
    is_active     boolean      NOT NULL DEFAULT true,
    is_superuser  boolean      NOT NULL DEFAULT false,
    created_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY ix_users_username (username),
    -- MySQL 唯一索引允许多个 NULL，天然等价 PG 的部分唯一索引
    UNIQUE KEY ix_users_email (email)
);

CREATE TABLE project_groups (
    id         char(32)     PRIMARY KEY,
    name       varchar(50)  NOT NULL,
    owner_id   char(32)     NOT NULL,
    sort_order int          NOT NULL DEFAULT 0,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_project_groups_owner_id (owner_id),
    CONSTRAINT fk_pg_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE projects (
    id          char(32)     PRIMARY KEY,
    name        varchar(100) NOT NULL,
    description text,
    owner_id    char(32)     NOT NULL,
    group_id    char(32),
    status      varchar(20)  NOT NULL DEFAULT 'active',
    is_deleted  boolean      NOT NULL DEFAULT false,
    deleted_at  datetime(6),
    created_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_projects_owner_id (owner_id),
    KEY ix_projects_group_id (group_id),
    CONSTRAINT fk_proj_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_proj_group FOREIGN KEY (group_id) REFERENCES project_groups (id) ON DELETE SET NULL
);

CREATE TABLE folders (
    id         char(32)     PRIMARY KEY,
    name       varchar(100) NOT NULL,
    parent_id  char(32),
    project_id char(32)     NOT NULL,
    owner_id   char(32)     NOT NULL,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_folders_parent (parent_id),
    CONSTRAINT fk_folder_parent FOREIGN KEY (parent_id) REFERENCES folders (id) ON DELETE CASCADE,
    CONSTRAINT fk_folder_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
    CONSTRAINT fk_folder_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE documents (
    id         char(32)     PRIMARY KEY,
    name       varchar(255) NOT NULL,
    type       varchar(100) NOT NULL,
    size       bigint       NOT NULL DEFAULT 0,
    status     varchar(20)  NOT NULL DEFAULT 'uploaded',
    s3_key     varchar(500) NOT NULL,
    remark     text,
    project_id char(32)     NOT NULL,
    folder_id  char(32),
    owner_id   char(32)     NOT NULL,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_documents_project (project_id),
    KEY ix_documents_folder (folder_id),
    CONSTRAINT fk_doc_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
    CONSTRAINT fk_doc_folder FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE SET NULL,
    CONSTRAINT fk_doc_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE latex_files (
    id            char(32)     PRIMARY KEY,
    project_id    char(32)     NOT NULL,
    file_path     varchar(500) NOT NULL,
    content_type  varchar(100),
    size_bytes    bigint       NOT NULL DEFAULT 0,
    content_hash  varchar(64),
    s3_key        varchar(500),
    last_saved_at datetime(6),
    created_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_project_file_path (project_id, file_path),
    KEY ix_latex_files_project_id (project_id),
    KEY ix_latex_files_content_hash (content_hash),
    CONSTRAINT fk_lf_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
);

CREATE TABLE compile_jobs (
    id         char(32)     PRIMARY KEY,
    project_id char(32)     NOT NULL,
    user_id    char(32)     NOT NULL,
    status     varchar(20)  NOT NULL DEFAULT 'pending',
    root_file  varchar(500) NOT NULL DEFAULT 'main.tex',
    compiler   varchar(20)  NOT NULL DEFAULT 'pdflatex',
    build_id   varchar(100),
    error      text,
    logs       text,
    sync_state json,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    KEY ix_compile_jobs_project_id (project_id),
    KEY ix_compile_jobs_user_id (user_id),
    KEY ix_compile_jobs_status (status),
    CONSTRAINT fk_cj_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
    CONSTRAINT fk_cj_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE user_settings (
    user_id    char(32)     NOT NULL,
    category   varchar(100) NOT NULL,
    settings   json         NOT NULL,
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id, category),
    CONSTRAINT fk_us_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
