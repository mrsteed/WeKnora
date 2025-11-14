-- Add FAQ knowledge base type and chunk metadata support
ALTER TABLE knowledge_bases
  ADD COLUMN IF NOT EXISTS type VARCHAR(32) NOT NULL DEFAULT 'document',
  ADD COLUMN IF NOT EXISTS faq_config JSONB;

UPDATE knowledge_bases
SET type = 'document'
WHERE type IS NULL OR type = '';

ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS metadata JSONB;

