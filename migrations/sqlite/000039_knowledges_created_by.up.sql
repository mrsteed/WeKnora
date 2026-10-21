ALTER TABLE knowledges ADD COLUMN created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_tenant_created_by
    ON knowledges (tenant_id, created_by);