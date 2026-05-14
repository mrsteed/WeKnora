ALTER TABLE chat_document_artifacts ADD COLUMN document_task_kind VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE chat_document_artifacts ADD COLUMN source_title VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE chat_document_artifacts ADD COLUMN target_language VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE chat_document_artifacts ADD COLUMN output_format VARCHAR(32) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_document_task_kind
    ON chat_document_artifacts(document_task_kind);