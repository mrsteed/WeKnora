DROP INDEX IF EXISTS idx_custom_agents_created_by;
DROP INDEX IF EXISTS idx_custom_agents_organization_id;
DROP INDEX IF EXISTS idx_custom_agents_visibility;

ALTER TABLE custom_agents DROP COLUMN IF EXISTS organization_id;
ALTER TABLE custom_agents DROP COLUMN IF EXISTS visibility;

DROP INDEX IF EXISTS idx_knowledge_bases_organization_id;
DROP INDEX IF EXISTS idx_knowledge_bases_visibility;

ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS organization_id;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS visibility;