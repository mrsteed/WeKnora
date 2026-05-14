DROP INDEX IF EXISTS idx_chat_document_artifacts_generation_status;

ALTER TABLE chat_document_artifacts
    DROP COLUMN IF EXISTS document_generation_status;
