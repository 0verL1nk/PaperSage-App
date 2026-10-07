-- 写作工作台底座：项目场景与持久化写作状态。
ALTER TABLE projects ADD COLUMN scenario varchar(32) NOT NULL DEFAULT 'undergraduate_thesis';

CREATE TABLE project_writing_states (
    project_id uuid PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    brief json NOT NULL,
    requirements json NOT NULL,
    phases json NOT NULL,
    current_phase varchar(20) NOT NULL DEFAULT 'prepare',
    revision int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX ix_project_writing_states_deleted_at ON project_writing_states (deleted_at);
