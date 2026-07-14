-- Migration: 000085_im_channels_created_by
-- Description: add creator ownership to IM channels so list visibility can be
-- isolated to the member who created each channel.

ALTER TABLE im_channels ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_created_by
    ON im_channels (tenant_id, created_by);

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_agent_created_by
    ON im_channels (tenant_id, agent_id, created_by);

COMMENT ON COLUMN im_channels.created_by IS
    'User ID of the tenant member who created this IM channel; list visibility is scoped to this owner.';

-- Best-effort backfill from custom agent ownership first.
UPDATE im_channels c
SET created_by = a.created_by
FROM custom_agents a
WHERE c.created_by = ''
  AND c.agent_id = a.id
  AND c.tenant_id = a.tenant_id
  AND a.deleted_at IS NULL
  AND COALESCE(a.created_by, '') <> '';

-- Fallback: bind any remaining historical channels to the earliest active tenant owner.
WITH owner_rows AS (
    SELECT DISTINCT ON (tenant_id) tenant_id, user_id
    FROM tenant_members
    WHERE role = 'owner'
      AND status = 'active'
      AND deleted_at IS NULL
    ORDER BY tenant_id, joined_at ASC, id ASC
)
UPDATE im_channels c
SET created_by = o.user_id
FROM owner_rows o
WHERE c.created_by = ''
  AND c.tenant_id = o.tenant_id;