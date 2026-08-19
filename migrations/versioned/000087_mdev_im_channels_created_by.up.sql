-- Migration: 000087_mdev_im_channels_created_by
-- Description: add creator ownership for IM channels.

ALTER TABLE im_channels
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_created_by
    ON im_channels (tenant_id, created_by);

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_agent_created_by
    ON im_channels (tenant_id, agent_id, created_by);

COMMENT ON COLUMN im_channels.created_by IS
    'Creator/owner of the IM channel inside a tenant. Used for per-user channel isolation in mdev.';

DO $$ BEGIN RAISE NOTICE '[Migration 000087] mdev IM channel ownership ready'; END $$;