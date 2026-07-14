DROP INDEX IF EXISTS idx_im_channels_tenant_agent_created_by;
DROP INDEX IF EXISTS idx_im_channels_tenant_created_by;
ALTER TABLE im_channels DROP COLUMN created_by;