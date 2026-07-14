-- Migration: 000086_knowledge_created_by
-- Description: track which internal account created or uploaded a knowledge item.

ALTER TABLE knowledges ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_tenant_created_by
    ON knowledges (tenant_id, created_by);

COMMENT ON COLUMN knowledges.created_by IS
    'Internal user ID that uploaded or created this knowledge item. Historical rows may remain empty when the original actor cannot be recovered safely.';