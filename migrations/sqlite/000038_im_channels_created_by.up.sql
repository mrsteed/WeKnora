ALTER TABLE im_channels ADD COLUMN created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_created_by
    ON im_channels (tenant_id, created_by);

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_agent_created_by
    ON im_channels (tenant_id, agent_id, created_by);