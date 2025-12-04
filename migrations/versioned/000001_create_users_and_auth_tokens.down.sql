-- 000001_create_users_and_auth_tokens.down.sql
-- Drop users and auth_tokens tables

BEGIN;

-- Drop foreign key constraints first
ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS fk_auth_tokens_user;
ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_users_tenant;

-- Drop tables
DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS users;

COMMIT;
