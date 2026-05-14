ALTER TABLE chat_document_artifacts
    ADD COLUMN IF NOT EXISTS document_generation_status VARCHAR(32) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_generation_status
    ON chat_document_artifacts(document_generation_status);
