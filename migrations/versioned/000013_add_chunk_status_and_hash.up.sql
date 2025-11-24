-- 000013_add_chunk_status_and_hash.up.sql
-- Add status and content_hash fields to chunks table

BEGIN;

-- Add status field to track chunk processing state
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS status INT NOT NULL DEFAULT 0;

-- Add content_hash field for quick content matching (primarily for FAQ)
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS content_hash VARCHAR(64);

-- Create index on content_hash for efficient lookup
CREATE INDEX IF NOT EXISTS idx_chunks_content_hash
    ON chunks(content_hash);

COMMIT;
