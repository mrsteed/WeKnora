-- Remove phone column from users table
DROP INDEX IF EXISTS idx_users_phone_unique;
ALTER TABLE users DROP COLUMN IF EXISTS phone;
