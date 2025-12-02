-- 000016_add_can_access_all_tenants.up.sql
-- Add can_access_all_tenants column to users table

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS can_access_all_tenants BOOLEAN NOT NULL DEFAULT FALSE;

COMMIT;

