-- 000013_add_chunk_status_and_hash.down.sql
-- Rollback: Remove status and content_hash fields from chunks table

BEGIN;

-- Drop index on content_hash
DROP INDEX IF EXISTS idx_chunks_content_hash;

-- Drop content_hash column
ALTER TABLE chunks
    DROP COLUMN IF EXISTS content_hash;

-- Drop status column
ALTER TABLE chunks
    DROP COLUMN IF EXISTS status;

COMMIT;
