DO $$
BEGIN
    IF to_regclass('public.organization_members') IS NOT NULL
       AND to_regclass('public.organization_members_pre_plan3') IS NULL THEN
        EXECUTE 'ALTER TABLE organization_members RENAME TO organization_members_pre_plan3';
    END IF;
END $$;

ALTER INDEX IF EXISTS idx_org_members_org_user RENAME TO idx_org_members_org_user_pre_plan3;
ALTER INDEX IF EXISTS idx_org_members_user_id RENAME TO idx_org_members_user_id_pre_plan3;
ALTER INDEX IF EXISTS idx_org_members_tenant_id RENAME TO idx_org_members_tenant_id_pre_plan3;
ALTER INDEX IF EXISTS idx_org_members_role RENAME TO idx_org_members_role_pre_plan3;