-- Migration: 000088_builtin_agents_global_visibility (rollback)
-- Description: Revert the visibility repair for persisted built-in agent
--              override rows (back to the schema column default).
--
--              NOTE: This is best-effort. It cannot distinguish rows whose
--              visibility was previously the explicit GORM default 'private'
--              from rows that may have been intentionally set to another
--              value; per the original bug, all pre-fix override rows are
--              expected to be 'private' (empty created_by), so restore that.

UPDATE custom_agents
SET visibility = 'private'
WHERE is_builtin
  AND deleted_at IS NULL
  AND visibility = 'global';

DO $$ BEGIN RAISE NOTICE '[Migration 000088 DOWN] built-in agents visibility reverted to private'; END $$;
