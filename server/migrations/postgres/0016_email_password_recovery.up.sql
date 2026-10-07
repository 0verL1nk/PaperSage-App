ALTER TABLE users ADD COLUMN auth_version bigint NOT NULL DEFAULT 0;
ALTER TABLE verification_tokens ADD COLUMN purpose varchar(32) NOT NULL DEFAULT 'email_verify';
