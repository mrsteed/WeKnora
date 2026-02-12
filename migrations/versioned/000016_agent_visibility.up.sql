-- Add visibility and organization_id to custom_agents table
ALTER TABLE custom_agents ADD COLUMN IF NOT EXISTS visibility VARCHAR(20) DEFAULT 'private';
ALTER TABLE custom_agents ADD COLUMN IF NOT EXISTS organization_id VARCHAR(36) DEFAULT NULL;

-- Indexes for visibility queries
CREATE INDEX IF NOT EXISTS idx_custom_agents_visibility ON custom_agents(visibility);
CREATE INDEX IF NOT EXISTS idx_custom_agents_organization_id ON custom_agents(organization_id);
CREATE INDEX IF NOT EXISTS idx_custom_agents_created_by ON custom_agents(created_by);

-- Migrate existing agents: set all existing custom agents to 'global' visibility
-- so they remain visible to all users after migration
UPDATE custom_agents SET visibility = 'global' WHERE visibility = 'private' AND is_builtin = false;
