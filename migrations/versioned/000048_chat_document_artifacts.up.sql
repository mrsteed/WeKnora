CREATE TABLE IF NOT EXISTS chat_document_artifacts (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    source_message_id VARCHAR(36) NOT NULL,
    source_request_id VARCHAR(64) NOT NULL DEFAULT '',
    parent_artifact_id VARCHAR(36) NOT NULL DEFAULT '',
    revision_no INTEGER NOT NULL DEFAULT 1,
    title VARCHAR(255) NOT NULL DEFAULT '',
    artifact_kind VARCHAR(32) NOT NULL,
    content_type VARCHAR(64) NOT NULL DEFAULT 'text/plain',
    content_snapshot TEXT NOT NULL,
    content_checksum VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'available',
    completion_status VARCHAR(32) NOT NULL DEFAULT '',
    operation VARCHAR(32) NOT NULL DEFAULT 'create',
    created_by VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITHOUT TIME ZONE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_document_artifacts_source_message_id
    ON chat_document_artifacts(source_message_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_tenant_session_created_at
    ON chat_document_artifacts(tenant_id, session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_tenant_parent_revision
    ON chat_document_artifacts(tenant_id, parent_artifact_id, revision_no);
CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_content_checksum
    ON chat_document_artifacts(content_checksum);
CREATE INDEX IF NOT EXISTS idx_chat_document_artifacts_deleted_at
    ON chat_document_artifacts(deleted_at);