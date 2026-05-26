DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_anonymous_visitor_id;
DROP INDEX IF EXISTS idx_sessions_agent_page_share_id;
DROP INDEX IF EXISTS idx_sessions_access_mode;

ALTER TABLE sessions DROP COLUMN expires_at;
ALTER TABLE sessions DROP COLUMN user_agent_hash;
ALTER TABLE sessions DROP COLUMN visitor_ip_hash;
ALTER TABLE sessions DROP COLUMN visitor_token_hash;
ALTER TABLE sessions DROP COLUMN anonymous_visitor_id;
ALTER TABLE sessions DROP COLUMN agent_page_share_id;
ALTER TABLE sessions DROP COLUMN access_mode;
