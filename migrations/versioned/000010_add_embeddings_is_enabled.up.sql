-- 000010_add_embeddings_is_enabled.up.sql
-- Add is_enabled column to embeddings table

BEGIN;

-- Add is_enabled column to embeddings table
ALTER TABLE embeddings
    ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN DEFAULT TRUE;

-- Create index for is_enabled column
CREATE INDEX IF NOT EXISTS idx_embeddings_is_enabled
    ON embeddings(is_enabled);

COMMIT;

