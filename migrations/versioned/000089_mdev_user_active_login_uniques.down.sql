DROP INDEX IF EXISTS idx_users_phone_active_unique;
DROP INDEX IF EXISTS idx_users_email_active_unique;
DROP INDEX IF EXISTS idx_users_username_active_unique;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_username_key') THEN
        ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_key') THEN
        ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_unique
    ON users (phone)
    WHERE phone IS NOT NULL AND btrim(phone) <> '';