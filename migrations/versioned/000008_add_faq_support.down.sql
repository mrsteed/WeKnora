-- 08-add-faq-support rollback
BEGIN;

ALTER TABLE chunks
  DROP COLUMN IF EXISTS metadata;

ALTER TABLE knowledge_bases
  DROP COLUMN IF EXISTS faq_config,
  DROP COLUMN IF EXISTS type;

COMMIT;

