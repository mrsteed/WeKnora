DROP INDEX IF EXISTS idx_chat_document_evidence_refs_source_span;

ALTER TABLE chat_document_evidence_refs DROP COLUMN source_end_at;
ALTER TABLE chat_document_evidence_refs DROP COLUMN source_start_at;
ALTER TABLE chat_document_evidence_refs DROP COLUMN excerpt;