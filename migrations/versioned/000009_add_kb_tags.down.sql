-- 000009_add_kb_tags.down.sql
-- Drop knowledge base scoped tags and tag_id fields

BEGIN;

-- Drop indexes and columns referencing tags
DROP INDEX IF EXISTS idx_chunks_tag;
ALTER TABLE chunks DROP COLUMN IF EXISTS tag_id;

DROP INDEX IF EXISTS idx_knowledges_tag;
ALTER TABLE knowledges DROP COLUMN IF EXISTS tag_id;

-- Drop tag table
DROP INDEX IF EXISTS idx_knowledge_tags_kb_name;
DROP INDEX IF EXISTS idx_knowledge_tags_kb;
DROP TABLE IF EXISTS knowledge_tags;

COMMIT;


