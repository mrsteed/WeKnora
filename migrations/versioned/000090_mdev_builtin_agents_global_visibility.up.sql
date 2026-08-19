-- Migration: 000090_mdev_builtin_agents_global_visibility
-- Description: optional safety repair for legacy/imported built-in agent override rows.

UPDATE custom_agents
   SET visibility = 'global'
 WHERE is_builtin
   AND deleted_at IS NULL
   AND COALESCE(visibility, '') <> 'global';

DO $$ BEGIN RAISE NOTICE '[Migration 000090] mdev built-in agent visibility repair applied'; END $$;