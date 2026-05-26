CREATE TABLE IF NOT EXISTS agent_page_shares (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    source_tenant_id INTEGER NOT NULL,
    share_code TEXT NOT NULL,
    access_scope TEXT NOT NULL DEFAULT 'anonymous',
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT NOT NULL,
    anonymous_session_limit INTEGER NOT NULL DEFAULT 0,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 0,
    last_accessed_at DATETIME,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    FOREIGN KEY (agent_id, source_tenant_id) REFERENCES custom_agents(id, tenant_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_page_shares_share_code
    ON agent_page_shares(share_code);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_page_shares_agent_tenant
    ON agent_page_shares(agent_id, source_tenant_id);

CREATE INDEX IF NOT EXISTS idx_agent_page_shares_status
    ON agent_page_shares(status);

CREATE INDEX IF NOT EXISTS idx_agent_page_shares_deleted_at
    ON agent_page_shares(deleted_at);
