-- Migration: 000088_mdev_knowledges_created_by
-- Description: track the uploader/creator of individual knowledge documents.

ALTER TABLE knowledges
    ADD COLUMN IF NOT EXISTS created_by VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_tenant_created_by
    ON knowledges (tenant_id, created_by);

COMMENT ON COLUMN knowledges.created_by IS
    'Internal user ID that uploaded or created this knowledge item. Distinct from knowledge_bases.creator_id.';

DO $$ BEGIN RAISE NOTICE '[Migration 000088] mdev knowledge document ownership ready'; END $$;