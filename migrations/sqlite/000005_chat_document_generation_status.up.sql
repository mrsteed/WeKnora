ALTER TABLE chat_document_artifacts
    ADD COLUMN document_generation_status TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_generation_status
    ON chat_document_artifacts(document_generation_status);
