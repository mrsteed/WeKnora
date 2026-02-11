-- 000013_org_tree_kb_visibility.down.sql
-- 回滚组织树 + 知识库可见性扩展

-- ============================================================
-- 3. knowledge_bases 表回滚
-- ============================================================
DROP INDEX IF EXISTS idx_knowledge_bases_organization_id;
DROP INDEX IF EXISTS idx_knowledge_bases_visibility;
DROP INDEX IF EXISTS idx_knowledge_bases_created_by;

ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS organization_id;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS visibility;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS created_by;

-- ============================================================
-- 2. users 表回滚
-- ============================================================
ALTER TABLE users DROP COLUMN IF EXISTS is_super_admin;

-- ============================================================
-- 1. organizations 表回滚
-- ============================================================
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS fk_organizations_parent;

DROP INDEX IF EXISTS idx_organizations_tenant_id;
DROP INDEX IF EXISTS idx_organizations_path;
DROP INDEX IF EXISTS idx_organizations_parent_id;

ALTER TABLE organizations DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS sort_order;
ALTER TABLE organizations DROP COLUMN IF EXISTS level;
ALTER TABLE organizations DROP COLUMN IF EXISTS path;
ALTER TABLE organizations DROP COLUMN IF EXISTS parent_id;
