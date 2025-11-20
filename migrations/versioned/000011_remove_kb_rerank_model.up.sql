-- 000011_remove_kb_rerank_model.up.sql
-- Remove rerank_model_id column from knowledge_bases table

BEGIN;

ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS rerank_model_id;

COMMIT;


