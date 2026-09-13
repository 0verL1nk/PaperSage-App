CREATE TABLE refresh_tokens (
    token_hash  varchar(64)  PRIMARY KEY,
    user_id     char(32)     NOT NULL,
    expires_at  datetime(6)  NOT NULL,
    revoked_at  datetime(6),
    created_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    KEY ix_refresh_tokens_user_id (user_id),
    CONSTRAINT fk_rt_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
