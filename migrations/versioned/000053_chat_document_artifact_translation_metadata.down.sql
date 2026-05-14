DROP INDEX IF EXISTS idx_chat_document_artifacts_document_task_kind;

ALTER TABLE chat_document_artifacts
    DROP COLUMN IF EXISTS output_format,
    DROP COLUMN IF EXISTS target_language,
    DROP COLUMN IF EXISTS source_title,
    DROP COLUMN IF EXISTS document_task_kind;