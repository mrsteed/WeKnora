-- 000011_remove_kb_rerank_model.down.sql
-- Reintroduce rerank_model_id column to knowledge_bases table

BEGIN;

ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS rerank_model_id VARCHAR(64);

COMMIT;


