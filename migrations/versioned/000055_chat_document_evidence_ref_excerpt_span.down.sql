DROP INDEX IF EXISTS idx_chat_document_evidence_refs_source_span;

ALTER TABLE chat_document_evidence_refs
    DROP COLUMN IF EXISTS source_end_at,
    DROP COLUMN IF EXISTS source_start_at,
    DROP COLUMN IF EXISTS excerpt;