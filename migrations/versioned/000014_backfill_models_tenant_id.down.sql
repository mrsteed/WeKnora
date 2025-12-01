-- 14-backfill-models-tenant-id.down.sql
-- Reset tenant_id in models back to 0 for records tied to knowledges entries

BEGIN;

UPDATE models m
JOIN knowledges k ON m.id = k.id
SET m.tenant_id = 0
WHERE m.tenant_id = k.tenant_id;

COMMIT;


