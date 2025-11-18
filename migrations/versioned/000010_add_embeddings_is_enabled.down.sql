-- 000010_add_embeddings_is_enabled.down.sql
-- Remove is_enabled column from embeddings table

BEGIN;

-- Drop index
DROP INDEX IF EXISTS idx_embeddings_is_enabled;

-- Remove is_enabled column
ALTER TABLE embeddings
    DROP COLUMN IF EXISTS is_enabled;

COMMIT;

