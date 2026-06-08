ALTER TABLE sessions ADD COLUMN IF NOT EXISTS access_mode VARCHAR(32) NOT NULL DEFAULT 'platform';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_page_share_id VARCHAR(36);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS anonymous_visitor_id VARCHAR(36);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS visitor_token_hash VARCHAR(64);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS visitor_ip_hash VARCHAR(64);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_agent_hash VARCHAR(64);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_sessions_access_mode ON sessions(access_mode);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_page_share_id ON sessions(agent_page_share_id);
CREATE INDEX IF NOT EXISTS idx_sessions_anonymous_visitor_id ON sessions(anonymous_visitor_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
