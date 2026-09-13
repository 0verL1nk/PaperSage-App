-- 幂等 baseline：全部 IF NOT EXISTS。交接既有库（schema 由 create_all 建好）时
-- 重放 0001 为 no-op；migrate 对预 stamp 版本的读取在两条驱动路径上行为
-- 不一致，幂等化使 stamp/未 stamp 两种情况殊途同归。
-- 0001_baseline.up.sql — PostgreSQL baseline schema
-- 真相来源：backend/models/*.py（SQLAlchemy 模型）。
-- 注意：Alembic 迁移链（4885f83a21bf/d94d978b402a 等）为 SQLite 形状的
-- 自动生成迁移（NUMERIC 主键），无法在 PostgreSQL 上干净重放；真实 PG
-- 库依赖 create_all 兜底建表，与模型一致，因此 baseline 直接对齐模型。

CREATE TABLE IF NOT EXISTS users (
    id            uuid PRIMARY KEY,
    username      varchar(50)  NOT NULL,
    password_hash varchar(255) NOT NULL,
    email         varchar(255),
    is_active     boolean      NOT NULL DEFAULT true,
    is_superuser  boolean      NOT NULL DEFAULT false,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ix_users_username ON users (username);

CREATE TABLE IF NOT EXISTS project_groups (
    id         uuid PRIMARY KEY,
    name       varchar(50) NOT NULL,
    owner_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    sort_order integer     NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_project_groups_owner_id ON project_groups (owner_id);

CREATE TABLE IF NOT EXISTS projects (
    id          uuid PRIMARY KEY,
    name        varchar(100) NOT NULL,
    description text,
    owner_id    uuid         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    group_id    uuid         REFERENCES project_groups (id) ON DELETE SET NULL,
    status      varchar(20)  NOT NULL DEFAULT 'active',
    is_deleted  boolean      NOT NULL DEFAULT false,
    deleted_at  timestamp,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_projects_owner_id ON projects (owner_id);
CREATE INDEX IF NOT EXISTS ix_projects_group_id ON projects (group_id);

CREATE TABLE IF NOT EXISTS folders (
    id         uuid PRIMARY KEY,
    name       varchar(100) NOT NULL,
    parent_id  uuid         REFERENCES folders (id) ON DELETE CASCADE,
    project_id uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    owner_id   uuid         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS documents (
    id         uuid PRIMARY KEY,
    name       varchar(255) NOT NULL,
    type       varchar(100) NOT NULL,
    size       bigint       NOT NULL DEFAULT 0,
    status     varchar(20)  NOT NULL DEFAULT 'uploaded',
    s3_key     varchar(500) NOT NULL,
    remark     text,
    project_id uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    folder_id  uuid         REFERENCES folders (id) ON DELETE SET NULL,
    owner_id   uuid         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS latex_files (
    id            uuid PRIMARY KEY,
    project_id    uuid         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    file_path     varchar(500) NOT NULL,
    content_type  varchar(100),
    size_bytes    bigint       NOT NULL DEFAULT 0,
    content_hash  varchar(64),
    s3_key        varchar(500),
    last_saved_at timestamptz,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT uq_project_file_path UNIQUE (project_id, file_path)
);
CREATE INDEX IF NOT EXISTS ix_latex_files_project_id ON latex_files (project_id);
CREATE INDEX IF NOT EXISTS ix_latex_files_content_hash ON latex_files (content_hash);

CREATE TABLE IF NOT EXISTS compile_jobs (
    id         uuid PRIMARY KEY,
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     varchar(20) NOT NULL DEFAULT 'pending',
    root_file  varchar(500) NOT NULL DEFAULT 'main.tex',
    compiler   varchar(20) NOT NULL DEFAULT 'pdflatex',
    build_id   varchar(100),
    error      text,
    logs       text,
    sync_state json,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_compile_jobs_project_id ON compile_jobs (project_id);
CREATE INDEX IF NOT EXISTS ix_compile_jobs_user_id ON compile_jobs (user_id);
CREATE INDEX IF NOT EXISTS ix_compile_jobs_status ON compile_jobs (status);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category   varchar(100) NOT NULL,
    settings   json        NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, category)
);
