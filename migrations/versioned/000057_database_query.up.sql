DO $$ BEGIN RAISE NOTICE '[Migration 000042] Creating database schema snapshot and audit tables'; END $$;

CREATE TABLE IF NOT EXISTS database_schema_snapshots (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    database_type VARCHAR(32) NOT NULL,
    database_name VARCHAR(255) NOT NULL,
    schema_name VARCHAR(255),
    schema_hash VARCHAR(128) NOT NULL,
    schema_json JSONB NOT NULL,
    refreshed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE NULL
);

CREATE TABLE IF NOT EXISTS database_table_columns (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    table_name VARCHAR(255) NOT NULL,
    column_name VARCHAR(255) NOT NULL,
    data_type VARCHAR(128) NOT NULL,
    nullable BOOLEAN NOT NULL DEFAULT true,
    comment TEXT,
    is_sensitive BOOLEAN NOT NULL DEFAULT false,
    ordinal_position INTEGER,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE NULL
);

CREATE TABLE IF NOT EXISTS database_query_audit_logs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    session_id VARCHAR(36),
    knowledge_base_id VARCHAR(36) NOT NULL,
    data_source_id VARCHAR(36) NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    original_sql TEXT NOT NULL,
    executed_sql TEXT,
    purpose TEXT,
    status VARCHAR(32) NOT NULL,
    row_count INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_db_schema_snapshots_tenant_kb
    ON database_schema_snapshots (tenant_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_db_schema_snapshots_tenant_ds
    ON database_schema_snapshots (tenant_id, data_source_id);
CREATE INDEX IF NOT EXISTS idx_db_schema_snapshots_refreshed_at
    ON database_schema_snapshots (refreshed_at);
CREATE INDEX IF NOT EXISTS idx_db_schema_snapshots_deleted_at
    ON database_schema_snapshots (deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS uk_db_schema_snapshots_active_ds
    ON database_schema_snapshots (tenant_id, data_source_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_db_table_columns_tenant_kb_table
    ON database_table_columns (tenant_id, knowledge_base_id, table_name);
CREATE INDEX IF NOT EXISTS idx_db_table_columns_tenant_ds
    ON database_table_columns (tenant_id, data_source_id);
CREATE INDEX IF NOT EXISTS idx_db_table_columns_deleted_at
    ON database_table_columns (deleted_at);

CREATE INDEX IF NOT EXISTS idx_db_query_audit_logs_tenant_kb
    ON database_query_audit_logs (tenant_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_db_query_audit_logs_tenant_ds
    ON database_query_audit_logs (tenant_id, data_source_id);
CREATE INDEX IF NOT EXISTS idx_db_query_audit_logs_created_at
    ON database_query_audit_logs (created_at);

DO $$ BEGIN RAISE NOTICE '[Migration 000042] Database schema snapshot and audit tables created successfully'; END $$;
