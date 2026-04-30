CREATE TABLE IF NOT EXISTS long_document_tasks (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    task_kind VARCHAR(32) NOT NULL,
    source_ref VARCHAR(255) NOT NULL DEFAULT '',
    source_snapshot_hash VARCHAR(64) NOT NULL DEFAULT '',
    output_format VARCHAR(32) NOT NULL DEFAULT 'markdown',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    total_batches INTEGER NOT NULL DEFAULT 0,
    completed_batches INTEGER NOT NULL DEFAULT 0,
    failed_batches INTEGER NOT NULL DEFAULT 0,
    artifact_path TEXT,
    artifact_id VARCHAR(36),
    error_message TEXT,
    task_options_json JSONB,
    idempotency_key VARCHAR(128) NOT NULL,
    retry_limit INTEGER NOT NULL DEFAULT 3,
    quality_status VARCHAR(32) NOT NULL DEFAULT '',
    created_by VARCHAR(36) NOT NULL DEFAULT '',
    completed_at TIMESTAMP,
    cancelled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_long_document_tasks_idempotency_key
    ON long_document_tasks(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_long_document_tasks_tenant_session
    ON long_document_tasks(tenant_id, session_id);
CREATE INDEX IF NOT EXISTS idx_long_document_tasks_tenant_status
    ON long_document_tasks(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_long_document_tasks_tenant_knowledge
    ON long_document_tasks(tenant_id, knowledge_id);

CREATE TABLE IF NOT EXISTS long_document_task_batches (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    task_id VARCHAR(36) NOT NULL,
    batch_no INTEGER NOT NULL,
    chunk_start_seq INTEGER NOT NULL DEFAULT 0,
    chunk_end_seq INTEGER NOT NULL DEFAULT 0,
    input_snapshot TEXT,
    output_payload TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    input_token_estimate INTEGER NOT NULL DEFAULT 0,
    output_token_estimate INTEGER NOT NULL DEFAULT 0,
    model_name VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    quality_status VARCHAR(32) NOT NULL DEFAULT '',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_long_document_task_batches_task_batch_no
    ON long_document_task_batches(task_id, batch_no);
CREATE INDEX IF NOT EXISTS idx_long_document_task_batches_tenant_task
    ON long_document_task_batches(tenant_id, task_id);
CREATE INDEX IF NOT EXISTS idx_long_document_task_batches_tenant_status
    ON long_document_task_batches(tenant_id, status);

CREATE TABLE IF NOT EXISTS long_document_artifacts (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    task_id VARCHAR(36) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_path TEXT NOT NULL,
    file_type VARCHAR(64) NOT NULL DEFAULT 'text/markdown',
    file_size BIGINT NOT NULL DEFAULT 0,
    checksum VARCHAR(64) NOT NULL DEFAULT '',
    storage_backend VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_long_document_artifacts_task_id
    ON long_document_artifacts(task_id);
CREATE INDEX IF NOT EXISTS idx_long_document_artifacts_tenant_status
    ON long_document_artifacts(tenant_id, status);
