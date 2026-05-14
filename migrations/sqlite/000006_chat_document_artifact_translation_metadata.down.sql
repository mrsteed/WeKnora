DROP INDEX IF EXISTS idx_chat_document_artifacts_document_task_kind;

ALTER TABLE chat_document_artifacts DROP COLUMN output_format;
ALTER TABLE chat_document_artifacts DROP COLUMN target_language;
ALTER TABLE chat_document_artifacts DROP COLUMN source_title;
ALTER TABLE chat_document_artifacts DROP COLUMN document_task_kind;