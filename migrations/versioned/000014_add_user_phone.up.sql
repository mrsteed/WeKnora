-- Add phone column to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20);

-- Add unique index for phone (partial index: only index non-null, non-empty values)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_unique ON users (phone) WHERE phone IS NOT NULL AND phone != '';
