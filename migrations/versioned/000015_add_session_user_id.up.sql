-- Add user_id to sessions table to support per-user session isolation
-- Previously sessions were only filtered by tenant_id, which means users sharing
-- the same tenant could see each other's conversations.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_id VARCHAR(36);

-- Backfill: for existing sessions, try to set user_id from the first user who belongs
-- to the same tenant. This is a best-effort migration for existing data.
UPDATE sessions s
SET user_id = (
    SELECT u.id FROM users u WHERE u.tenant_id = s.tenant_id LIMIT 1
)
WHERE s.user_id IS NULL;

-- Create index for efficient user-level session queries
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_user ON sessions(tenant_id, user_id);
