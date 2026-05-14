DROP INDEX IF EXISTS idx_chat_document_artifacts_generation_status;

ALTER TABLE chat_document_artifacts
    DROP COLUMN document_generation_status;
