-- Migration: 000122_mdev_custom_agents_visibility
-- Description: add same-tenant visibility columns for custom agents and normalize built-in overrides.

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

UPDATE custom_agents
   SET visibility = 'global',
       organization_id = NULL
 WHERE is_builtin
   AND deleted_at IS NULL
   AND (COALESCE(visibility, '') <> 'global' OR COALESCE(organization_id, '') <> '');

DO $$ BEGIN RAISE NOTICE '[Migration 000122] mdev custom agent visibility columns ready'; END $$;