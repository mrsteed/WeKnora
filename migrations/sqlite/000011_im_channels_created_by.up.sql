ALTER TABLE im_channels ADD COLUMN created_by TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_created_by
    ON im_channels (tenant_id, created_by);

CREATE INDEX IF NOT EXISTS idx_im_channels_tenant_agent_created_by
    ON im_channels (tenant_id, agent_id, created_by);

-- Best-effort backfill from custom agent ownership first.
UPDATE im_channels
SET created_by = (
    SELECT a.created_by
    FROM custom_agents a
    WHERE a.id = im_channels.agent_id
      AND a.tenant_id = im_channels.tenant_id
      AND a.deleted_at IS NULL
      AND IFNULL(a.created_by, '') <> ''
    LIMIT 1
)
WHERE created_by = ''
  AND EXISTS (
    SELECT 1
    FROM custom_agents a
    WHERE a.id = im_channels.agent_id
      AND a.tenant_id = im_channels.tenant_id
      AND a.deleted_at IS NULL
      AND IFNULL(a.created_by, '') <> ''
  );

-- Fallback: bind remaining historical channels to the earliest active tenant owner.
UPDATE im_channels
SET created_by = (
    SELECT tm.user_id
    FROM tenant_members tm
    WHERE tm.tenant_id = im_channels.tenant_id
      AND tm.role = 'owner'
      AND tm.status = 'active'
      AND tm.deleted_at IS NULL
    ORDER BY tm.joined_at ASC, tm.id ASC
    LIMIT 1
)
WHERE created_by = '';