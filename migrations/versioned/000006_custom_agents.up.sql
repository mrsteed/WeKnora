-- Migration: 000006_custom_agents
-- Description: Add custom agents table for GPTs-like agent configuration
DO $$ BEGIN RAISE NOTICE '[Migration 000006] Starting custom agents setup...'; END $$;

-- Create custom_agents table
DO $$ BEGIN RAISE NOTICE '[Migration 000006] Creating table: custom_agents'; END $$;
CREATE TABLE IF NOT EXISTS custom_agents (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    avatar VARCHAR(64),
    is_builtin BOOLEAN NOT NULL DEFAULT false,
    type VARCHAR(32) NOT NULL DEFAULT 'custom',
    tenant_id INTEGER NOT NULL,
    created_by VARCHAR(36),
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Add indexes for custom_agents
CREATE INDEX IF NOT EXISTS idx_custom_agents_tenant_id ON custom_agents(tenant_id);
CREATE INDEX IF NOT EXISTS idx_custom_agents_type ON custom_agents(type);
CREATE INDEX IF NOT EXISTS idx_custom_agents_is_builtin ON custom_agents(is_builtin);
CREATE INDEX IF NOT EXISTS idx_custom_agents_deleted_at ON custom_agents(deleted_at);

-- Add agent_id column to sessions table to track which agent was used
DO $$ BEGIN RAISE NOTICE '[Migration 000006] Adding agent_id column to sessions table'; END $$;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_id VARCHAR(36);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_id ON sessions(agent_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000006] Custom agents setup completed successfully!'; END $$;
