-- 邮箱验证码登录：一次性凭据只存 bcrypt 哈希，无明文。
-- 不绑 user_id——首次登录时用户还不存在；按 email 取最新未消费行。
CREATE TABLE email_login_codes (
    id uuid PRIMARY KEY,
    email varchar(255) NOT NULL,
    code_hash varchar(255) NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_email_login_codes_email ON email_login_codes (email, created_at);
