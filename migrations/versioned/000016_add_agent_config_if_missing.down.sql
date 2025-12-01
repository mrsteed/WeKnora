-- 000016_add_agent_config_if_missing.down.sql
-- Rollback for agent_config / context_config / agent_steps columns and indexes
BEGIN;
-- Drop JSONB indexes if they exist
DROP INDEX IF EXISTS idx_messages_agent_steps;
DROP INDEX IF EXISTS idx_sessions_context_config;
DROP INDEX IF EXISTS idx_sessions_agent_config;
DROP INDEX IF EXISTS idx_tenants_agent_config;
-- Drop columns if they exist
ALTER TABLE messages
    DROP COLUMN IF EXISTS agent_steps;
ALTER TABLE sessions
    DROP COLUMN IF EXISTS context_config;
ALTER TABLE sessions
    DROP COLUMN IF EXISTS agent_config;
ALTER TABLE tenants
    DROP COLUMN IF EXISTS agent_config;
COMMIT;



