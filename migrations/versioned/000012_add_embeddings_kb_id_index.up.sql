-- 000012_add_embeddings_kb_id_index.up.sql
-- Add B-tree index on knowledge_base_id for embeddings table to improve query performance

BEGIN;

-- Create index for knowledge_base_id to optimize filtering queries
CREATE INDEX IF NOT EXISTS idx_embeddings_knowledge_base_id
    ON embeddings(knowledge_base_id);

COMMIT;





