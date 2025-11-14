-- 07-add-builtin-models.sql (rollback)
-- Remove is_builtin field from models table

BEGIN;

-- Drop index
DROP INDEX IF EXISTS idx_models_is_builtin;

-- Remove is_builtin column
ALTER TABLE models 
DROP COLUMN IF EXISTS is_builtin;

COMMIT;

