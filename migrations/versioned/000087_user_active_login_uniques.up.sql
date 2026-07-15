ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

DROP INDEX IF EXISTS idx_users_phone_unique;
DROP INDEX IF EXISTS idx_users_username_active_unique;
DROP INDEX IF EXISTS idx_users_email_active_unique;
DROP INDEX IF EXISTS idx_users_phone_active_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_active_unique
    ON users (username)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active_unique
    ON users (email)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_active_unique
    ON users (phone)
    WHERE phone IS NOT NULL AND btrim(phone) <> '' AND deleted_at IS NULL;