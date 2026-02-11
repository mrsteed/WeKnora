-- Remove user_id from sessions table
DROP INDEX IF EXISTS idx_sessions_tenant_user;
DROP INDEX IF EXISTS idx_sessions_user_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS user_id;
