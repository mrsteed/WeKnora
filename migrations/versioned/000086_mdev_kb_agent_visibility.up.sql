-- Migration: 000086_mdev_kb_agent_visibility
-- Description: add same-tenant visibility columns for knowledge bases and custom agents.

ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(20) NOT NULL DEFAULT 'private';

ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS organization_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility
    ON knowledge_bases(visibility);

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_organization_id
    ON knowledge_bases(organization_id);

COMMENT ON COLUMN knowledge_bases.visibility IS
    'mdev same-tenant visibility: global | org | private';

COMMENT ON COLUMN knowledge_bases.organization_id IS
    'organization scope used when knowledge_bases.visibility = ''org''.';

ALTER TABLE custom_agents
    ADD COLUMN IF NOT EXISTS visibility VARCHAR(20) NOT NULL DEFAULT 'private';

ALTER TABLE custom_agents
    ADD COLUMN IF NOT EXISTS organization_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_custom_agents_visibility
    ON custom_agents(visibility);

CREATE INDEX IF NOT EXISTS idx_custom_agents_organization_id
    ON custom_agents(organization_id);

CREATE INDEX IF NOT EXISTS idx_custom_agents_created_by
    ON custom_agents(created_by);

COMMENT ON COLUMN custom_agents.visibility IS
    'mdev same-tenant visibility: global | org | private';

COMMENT ON COLUMN custom_agents.organization_id IS
    'organization scope used when custom_agents.visibility = ''org''.';

DO $$ BEGIN RAISE NOTICE '[Migration 000086] mdev KB/agent visibility columns ready'; END $$;