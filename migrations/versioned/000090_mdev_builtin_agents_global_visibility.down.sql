-- Best-effort rollback: only revert ownerless built-in override rows.

UPDATE custom_agents
   SET visibility = 'private'
 WHERE is_builtin
   AND deleted_at IS NULL
   AND COALESCE(created_by, '') = ''
   AND COALESCE(visibility, '') = 'global';

DO $$ BEGIN RAISE NOTICE '[Migration 000090 down] ownerless built-in agent rows restored to private'; END $$;