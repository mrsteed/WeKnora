ALTER TABLE chat_document_evidence_refs ADD COLUMN IF NOT EXISTS excerpt TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_document_evidence_refs ADD COLUMN IF NOT EXISTS source_start_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE chat_document_evidence_refs ADD COLUMN IF NOT EXISTS source_end_at BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_source_span
    ON chat_document_evidence_refs(source_start_at, source_end_at);