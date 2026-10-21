ALTER TABLE custom_agents ADD COLUMN visibility VARCHAR(20) NOT NULL DEFAULT 'private';

ALTER TABLE custom_agents ADD COLUMN organization_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_custom_agents_visibility
    ON custom_agents (visibility);

CREATE INDEX IF NOT EXISTS idx_custom_agents_organization_id
    ON custom_agents (organization_id);

CREATE INDEX IF NOT EXISTS idx_custom_agents_created_by
    ON custom_agents (created_by);

UPDATE custom_agents
   SET visibility = 'global',
       organization_id = NULL
 WHERE is_builtin = 1
   AND deleted_at IS NULL
   AND (COALESCE(visibility, '') <> 'global' OR COALESCE(organization_id, '') <> '');