-- 000016_add_can_access_all_tenants.down.sql
-- Remove can_access_all_tenants column from users table

BEGIN;

ALTER TABLE users
    DROP COLUMN IF EXISTS can_access_all_tenants;

COMMIT;

