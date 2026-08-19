-- Migration: 000085_mdev_org_tree_foundation
-- Description: add org-tree columns and platform super-admin flag for mdev.

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS parent_id VARCHAR(36);
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS path TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS level INTEGER NOT NULL DEFAULT 1;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS tenant_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_organizations_parent_id ON organizations(parent_id);
CREATE INDEX IF NOT EXISTS idx_organizations_path ON organizations USING btree(path text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_organizations_tenant_id ON organizations(tenant_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM information_schema.table_constraints
         WHERE constraint_name = 'fk_organizations_parent'
           AND table_name = 'organizations'
    ) THEN
        ALTER TABLE organizations
            ADD CONSTRAINT fk_organizations_parent
            FOREIGN KEY (parent_id) REFERENCES organizations(id) ON DELETE SET NULL;
    END IF;
END $$;

UPDATE organizations
   SET path = '/' || id
 WHERE COALESCE(path, '') = '';

ALTER TABLE users ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20);

UPDATE users
   SET is_super_admin = TRUE
 WHERE can_access_all_tenants = TRUE
   AND is_super_admin = FALSE;

COMMENT ON COLUMN organizations.tenant_id IS
    'mdev org-tree tenant scope; NULL keeps upstream shared-space rows compatible.';

COMMENT ON COLUMN users.is_super_admin IS
    'Local platform-level super admin flag used by org-tree and same-tenant visibility compatibility paths.';

COMMENT ON COLUMN users.phone IS
  'Optional phone number used by local personnel management and active-only uniqueness rules.';

DO $$ BEGIN RAISE NOTICE '[Migration 000085] mdev org-tree foundation ready'; END $$;