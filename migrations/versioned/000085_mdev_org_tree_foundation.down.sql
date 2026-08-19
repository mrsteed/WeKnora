ALTER TABLE organizations DROP CONSTRAINT IF EXISTS fk_organizations_parent;

DROP INDEX IF EXISTS idx_organizations_tenant_id;
DROP INDEX IF EXISTS idx_organizations_path;
DROP INDEX IF EXISTS idx_organizations_parent_id;

ALTER TABLE organizations DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS sort_order;
ALTER TABLE organizations DROP COLUMN IF EXISTS level;
ALTER TABLE organizations DROP COLUMN IF EXISTS path;
ALTER TABLE organizations DROP COLUMN IF EXISTS parent_id;

ALTER TABLE users DROP COLUMN IF EXISTS phone;
ALTER TABLE users DROP COLUMN IF EXISTS is_super_admin;