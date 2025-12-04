-- 000019_add_is_temporary_to_knowledge_bases.up.sql
-- Add is_temporary column to knowledge_bases table

BEGIN;

-- Add is_temporary column
ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS is_temporary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN knowledge_bases.is_temporary IS 'Whether this knowledge base is temporary (ephemeral) and should be hidden from UI';

COMMIT;
