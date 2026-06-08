CREATE TABLE IF NOT EXISTS agent_page_shares (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id VARCHAR(36) NOT NULL,
    source_tenant_id BIGINT NOT NULL,
    share_code VARCHAR(64) NOT NULL,
    access_scope VARCHAR(16) NOT NULL DEFAULT 'anonymous',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_by VARCHAR(36) NOT NULL,
    anonymous_session_limit INTEGER NOT NULL DEFAULT 0,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (agent_id, source_tenant_id) REFERENCES custom_agents(id, tenant_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_page_shares_share_code
    ON agent_page_shares(share_code)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_page_shares_agent_tenant
    ON agent_page_shares(agent_id, source_tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_page_shares_status
    ON agent_page_shares(status);

CREATE INDEX IF NOT EXISTS idx_agent_page_shares_deleted_at
    ON agent_page_shares(deleted_at);
