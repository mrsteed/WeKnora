-- Migration: 000091_mdev_org_tree_members_table_name
-- Description: restore the user-scoped org-tree membership table name to organization_members.

DO $$
BEGIN
    IF to_regclass('public.organization_members_pre_plan3') IS NOT NULL
       AND to_regclass('public.organization_members') IS NULL THEN
        EXECUTE 'ALTER TABLE organization_members_pre_plan3 RENAME TO organization_members';
    END IF;
END $$;

ALTER INDEX IF EXISTS idx_org_members_org_user_pre_plan3 RENAME TO idx_org_members_org_user;
ALTER INDEX IF EXISTS idx_org_members_user_id_pre_plan3 RENAME TO idx_org_members_user_id;
ALTER INDEX IF EXISTS idx_org_members_tenant_id_pre_plan3 RENAME TO idx_org_members_tenant_id;
ALTER INDEX IF EXISTS idx_org_members_role_pre_plan3 RENAME TO idx_org_members_role;

DO $$ BEGIN RAISE NOTICE '[Migration 000091] organization_members_pre_plan3 renamed to organization_members'; END $$;