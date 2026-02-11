-- 000013_org_tree_kb_visibility.up.sql
-- 组织树 + 知识库可见性扩展

-- ============================================================
-- 1. organizations 表扩展：组织树结构
-- ============================================================
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS parent_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS path TEXT DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS level INTEGER DEFAULT 1;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS sort_order INTEGER DEFAULT 0;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS tenant_id BIGINT DEFAULT NULL;

-- 索引
CREATE INDEX IF NOT EXISTS idx_organizations_parent_id ON organizations(parent_id);
CREATE INDEX IF NOT EXISTS idx_organizations_path ON organizations USING btree(path text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_organizations_tenant_id ON organizations(tenant_id);

-- 自引用外键（父组织）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_organizations_parent'
          AND table_name = 'organizations'
    ) THEN
        ALTER TABLE organizations ADD CONSTRAINT fk_organizations_parent
            FOREIGN KEY (parent_id) REFERENCES organizations(id) ON DELETE SET NULL;
    END IF;
END $$;

-- 回填现有组织的 path（格式: /self_id）
UPDATE organizations SET path = '/' || id WHERE path = '' OR path IS NULL;

-- ============================================================
-- 2. users 表扩展：超级管理员标记
-- ============================================================
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN DEFAULT false;

-- 从已有的 can_access_all_tenants 回填
UPDATE users SET is_super_admin = true WHERE can_access_all_tenants = true AND is_super_admin = false;

-- ============================================================
-- 3. knowledge_bases 表扩展：可见性 + 归属
-- ============================================================
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) DEFAULT '';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS visibility VARCHAR(20) DEFAULT 'private';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS organization_id VARCHAR(36) DEFAULT NULL;

-- 索引
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_created_by ON knowledge_bases(created_by);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility ON knowledge_bases(visibility);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_organization_id ON knowledge_bases(organization_id);
