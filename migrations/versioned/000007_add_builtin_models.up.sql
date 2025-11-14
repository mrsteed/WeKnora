-- 07-add-builtin-models.sql
-- Add is_builtin field to models table to support builtin models visible to all tenants

BEGIN;

-- Add is_builtin column to models table
ALTER TABLE models 
ADD COLUMN IF NOT EXISTS is_builtin BOOLEAN NOT NULL DEFAULT false;

-- Add index for is_builtin field to improve query performance
CREATE INDEX IF NOT EXISTS idx_models_is_builtin ON models(is_builtin);

COMMIT;

