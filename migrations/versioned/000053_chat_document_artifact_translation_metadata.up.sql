ALTER TABLE chat_document_artifacts
    ADD COLUMN IF NOT EXISTS document_task_kind VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_title VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_language VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS output_format VARCHAR(32) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_document_task_kind
    ON chat_document_artifacts(document_task_kind);