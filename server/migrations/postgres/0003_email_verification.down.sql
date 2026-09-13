DROP TABLE IF EXISTS verification_tokens;
DROP TABLE IF EXISTS system_settings;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
DROP INDEX IF EXISTS ix_users_email;
