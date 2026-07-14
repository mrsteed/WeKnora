DROP INDEX IF EXISTS idx_knowledges_tenant_created_by;
ALTER TABLE knowledges DROP COLUMN IF EXISTS created_by;