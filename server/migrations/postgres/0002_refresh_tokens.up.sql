-- 0002_refresh_tokens.up.sql — refresh token 注册表（DB 持久化）
-- 背景：此前注册表在进程内存（cache），重启即丢、登录态全失效。
-- token 本体不入库，只存 SHA-256 hex（token_hash），泄露库也不至于
-- 泄露可用凭据（且 token 是 JWE 密文、随机 IV，hash 不可逆推）。

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash  varchar(64) PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_refresh_tokens_user_id ON refresh_tokens (user_id);
