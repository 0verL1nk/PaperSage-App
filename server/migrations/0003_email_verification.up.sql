-- 0003：学校邮箱注册制 + 系统设置
-- users.email 升级为注册账号（唯一）；email_verified 准入门槛；
-- system_settings 为 admin 可改的运行时配置（如允许的邮箱域名白名单）。

CREATE UNIQUE INDEX IF NOT EXISTS ix_users_email ON users (email) WHERE email IS NOT NULL;

ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_email_registered boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS system_settings (
    key        varchar(100) PRIMARY KEY,
    value      json        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS verification_tokens (
    token_hash  varchar(64) PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz
);
