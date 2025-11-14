-- Add FAQ knowledge base type and chunk metadata storage
ALTER TABLE knowledge_bases
    ADD COLUMN type VARCHAR(32) NOT NULL DEFAULT 'document' AFTER name,
    ADD COLUMN faq_config JSON NULL AFTER extract_config;

UPDATE knowledge_bases SET type = 'document' WHERE type IS NULL OR type = '';

ALTER TABLE chunks
    ADD COLUMN metadata JSON NULL AFTER indirect_relation_chunks;

