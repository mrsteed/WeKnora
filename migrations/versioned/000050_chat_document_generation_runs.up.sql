CREATE TABLE IF NOT EXISTS chat_document_generation_runs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    root_message_id VARCHAR(36) NOT NULL DEFAULT '',
    root_artifact_id VARCHAR(36) NOT NULL DEFAULT '',
    agent_id VARCHAR(36) NOT NULL DEFAULT '',
    original_query TEXT NOT NULL,
    document_title VARCHAR(255) NOT NULL DEFAULT '',
    outline_json JSONB,
    effective_kb_ids_json JSONB,
    completed_sections_json JSONB,
    status VARCHAR(32) NOT NULL DEFAULT 'planning',
    auto_continue_round INTEGER NOT NULL DEFAULT 0,
    max_rounds INTEGER NOT NULL DEFAULT 8,
    model_id VARCHAR(128) NOT NULL DEFAULT '',
    created_by VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_document_generation_runs_tenant_session
    ON chat_document_generation_runs(tenant_id, session_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_generation_runs_tenant_status
    ON chat_document_generation_runs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_chat_document_generation_runs_root_message_id
    ON chat_document_generation_runs(root_message_id);
CREATE INDEX IF NOT EXISTS idx_chat_document_generation_runs_root_artifact_id
    ON chat_document_generation_runs(root_artifact_id);