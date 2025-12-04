-- 000019_add_is_temporary_to_knowledge_bases.down.sql
-- Remove is_temporary column from knowledge_bases table

BEGIN;

-- Drop is_temporary column
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS is_temporary;

COMMIT;
