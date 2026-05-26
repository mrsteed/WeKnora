ALTER TABLE sessions ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'platform';
ALTER TABLE sessions ADD COLUMN agent_page_share_id TEXT;
ALTER TABLE sessions ADD COLUMN anonymous_visitor_id TEXT;
ALTER TABLE sessions ADD COLUMN visitor_token_hash TEXT;
ALTER TABLE sessions ADD COLUMN visitor_ip_hash TEXT;
ALTER TABLE sessions ADD COLUMN user_agent_hash TEXT;
ALTER TABLE sessions ADD COLUMN expires_at DATETIME;

CREATE INDEX IF NOT EXISTS idx_sessions_access_mode ON sessions(access_mode);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_page_share_id ON sessions(agent_page_share_id);
CREATE INDEX IF NOT EXISTS idx_sessions_anonymous_visitor_id ON sessions(anonymous_visitor_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
