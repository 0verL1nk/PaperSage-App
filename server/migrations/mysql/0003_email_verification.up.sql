ALTER TABLE users ADD COLUMN is_email_registered boolean NOT NULL DEFAULT false;

CREATE TABLE verification_tokens (
    token_hash  varchar(64)  PRIMARY KEY,
    user_id     char(32)     NOT NULL,
    expires_at  datetime(6)  NOT NULL,
    used_at     datetime(6),
    KEY ix_vt_user (user_id),
    CONSTRAINT fk_vt_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE system_settings (
    `key`      varchar(100) PRIMARY KEY,
    `value`    json         NOT NULL,
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
);
