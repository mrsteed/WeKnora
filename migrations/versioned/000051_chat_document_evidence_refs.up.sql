CREATE TABLE IF NOT EXISTS chat_document_evidence_refs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    run_id VARCHAR(36) NOT NULL DEFAULT '',
    artifact_id VARCHAR(36) NOT NULL,
    message_id VARCHAR(36) NOT NULL DEFAULT '',
    section_id VARCHAR(128) NOT NULL DEFAULT '',
    section_heading VARCHAR(255) NOT NULL DEFAULT '',
    query TEXT NOT NULL DEFAULT '',
    knowledge_base_id VARCHAR(36) NOT NULL DEFAULT '',
    knowledge_id VARCHAR(36) NOT NULL DEFAULT '',
    chunk_id VARCHAR(128) NOT NULL DEFAULT '',
    source_title VARCHAR(255) NOT NULL DEFAULT '',
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    evidence_type VARCHAR(32) NOT NULL DEFAULT 'chunk',
    content_checksum VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_tenant_artifact
    ON chat_document_evidence_refs(tenant_id, artifact_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_run_id
    ON chat_document_evidence_refs(run_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_message_id
    ON chat_document_evidence_refs(message_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_chunk_id
    ON chat_document_evidence_refs(chunk_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_evidence_refs_content_checksum
    ON chat_document_evidence_refs(content_checksum);