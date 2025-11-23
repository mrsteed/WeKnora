-- 000012_add_embeddings_kb_id_index.down.sql
-- Remove B-tree index on knowledge_base_id from embeddings table

BEGIN;

DROP INDEX IF EXISTS idx_embeddings_knowledge_base_id;

COMMIT;





