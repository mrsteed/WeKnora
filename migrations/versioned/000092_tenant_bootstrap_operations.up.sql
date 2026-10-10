-- Resumable workspace bootstrap state for the post-create setup wizard.

CREATE TABLE IF NOT EXISTS tenant_bootstrap_operations (
    id                      VARCHAR(36) PRIMARY KEY,
    tenant_id               BIGINT NOT NULL,
    idempotency_key         VARCHAR(128) NOT NULL,
    mode                    VARCHAR(16) NOT NULL DEFAULT 'existing',
    status                  VARCHAR(24) NOT NULL DEFAULT 'pending',
    request_hash            VARCHAR(64) NOT NULL DEFAULT '',
    request_payload         JSONB,
    result_payload          JSONB,
    root_org_id             VARCHAR(36) NOT NULL DEFAULT '',
    root_org_name           VARCHAR(255) NOT NULL DEFAULT '',
    user_id                 VARCHAR(36) NOT NULL DEFAULT '',
    invite_url              TEXT NOT NULL DEFAULT '',
    tenant_membership_done  BOOLEAN NOT NULL DEFAULT FALSE,
    org_assignment_done     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_bootstrap_tenant_key
    ON tenant_bootstrap_operations (tenant_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_tenant_bootstrap_status
    ON tenant_bootstrap_operations (status);
