-- 14-backfill-models-tenant-id.up.sql
-- Backfill tenant_id in models table using knowledges records that share the same id

BEGIN;

WITH tenant_source AS (
    SELECT 
        k.id AS model_id,
        k.tenant_id
    FROM knowledges k
    WHERE k.tenant_id IS NOT NULL
)
UPDATE models m
JOIN tenant_source ts ON m.id = ts.model_id
SET m.tenant_id = ts.tenant_id
WHERE m.tenant_id = 0;

COMMIT;


