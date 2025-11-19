-- Remove rerank_model_id from knowledge_bases table
-- ReRank model is now configured globally in conversation settings, not per knowledge base
ALTER TABLE knowledge_bases DROP COLUMN rerank_model_id;
