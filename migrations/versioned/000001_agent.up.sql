BEGIN;

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    avatar VARCHAR(500),
    tenant_id INTEGER,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

COMMENT ON TABLE users IS 'User accounts in the system';
COMMENT ON COLUMN users.id IS 'Unique identifier of the user';
COMMENT ON COLUMN users.username IS 'Username of the user';
COMMENT ON COLUMN users.email IS 'Email address of the user';
COMMENT ON COLUMN users.password_hash IS 'Hashed password of the user';
COMMENT ON COLUMN users.avatar IS 'Avatar URL of the user';
COMMENT ON COLUMN users.tenant_id IS 'Tenant ID that the user belongs to';
COMMENT ON COLUMN users.is_active IS 'Whether the user is active';

-- Add indexes for users
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- Add foreign key constraint for tenant_id (only if not exists)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'fk_users_tenant'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT fk_users_tenant
            FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE SET NULL;
        RAISE NOTICE 'Added foreign key constraint fk_users_tenant';
    ELSE
        RAISE NOTICE 'Foreign key constraint fk_users_tenant already exists';
    END IF;
END $$;

-- Create auth_tokens table
CREATE TABLE IF NOT EXISTS auth_tokens (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(36) NOT NULL,
    token TEXT NOT NULL,
    token_type VARCHAR(50) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE auth_tokens IS 'Authentication tokens for users';
COMMENT ON COLUMN auth_tokens.id IS 'Unique identifier of the token';
COMMENT ON COLUMN auth_tokens.user_id IS 'User ID that owns this token';
COMMENT ON COLUMN auth_tokens.token IS 'Token value (JWT or other format)';
COMMENT ON COLUMN auth_tokens.token_type IS 'Token type (access_token, refresh_token)';
COMMENT ON COLUMN auth_tokens.expires_at IS 'Token expiration time';
COMMENT ON COLUMN auth_tokens.is_revoked IS 'Whether the token is revoked';

-- Add indexes for auth_tokens
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_token ON auth_tokens(token);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_token_type ON auth_tokens(token_type);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_expires_at ON auth_tokens(expires_at);

-- Add foreign key constraint (only if not exists)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'fk_auth_tokens_user'
    ) THEN
        ALTER TABLE auth_tokens
            ADD CONSTRAINT fk_auth_tokens_user
            FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
        RAISE NOTICE 'Added foreign key constraint fk_auth_tokens_user';
    ELSE
        RAISE NOTICE 'Foreign key constraint fk_auth_tokens_user already exists';
    END IF;
END $$;

-- Add is_temporary column to knowledge_bases
ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS is_temporary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN knowledge_bases.is_temporary IS 'Whether this knowledge base is temporary (ephemeral) and should be hidden from UI';

-- Add context_config column to tenants
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS context_config JSONB;

COMMENT ON COLUMN tenants.context_config IS 'Global Context configuration for this tenant (default for all sessions)';

-- Add conversation_config column to tenants
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS conversation_config JSONB;

COMMENT ON COLUMN tenants.conversation_config IS 'Global Conversation configuration for this tenant (default for normal mode sessions)';

-- For tenants table: add agent_config
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'tenants' 
        AND column_name = 'agent_config'
    ) THEN
        ALTER TABLE tenants 
        ADD COLUMN agent_config JSONB DEFAULT NULL;
        
        COMMENT ON COLUMN tenants.agent_config IS 'Tenant-level agent configuration in JSON format';
        
        RAISE NOTICE 'Added agent_config column to tenants table';
    ELSE
        -- If field exists but type is JSON, convert to JSONB
        IF EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_name = 'tenants' 
            AND column_name = 'agent_config'
            AND data_type = 'json'
        ) THEN
            ALTER TABLE tenants 
            ALTER COLUMN agent_config TYPE JSONB USING agent_config::jsonb;
            
            RAISE NOTICE 'Converted tenants.agent_config from JSON to JSONB';
        ELSE
            RAISE NOTICE 'agent_config column already exists in tenants table';
        END IF;
    END IF;
END $$;

-- For sessions table: add agent_config
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'sessions' 
        AND column_name = 'agent_config'
    ) THEN
        ALTER TABLE sessions 
        ADD COLUMN agent_config JSONB DEFAULT NULL;
        
        COMMENT ON COLUMN sessions.agent_config IS 'Session-level agent configuration in JSON format';
        
        RAISE NOTICE 'Added agent_config column to sessions table';
    ELSE
        RAISE NOTICE 'agent_config column already exists in sessions table';
    END IF;
END $$;

-- For sessions table: add context_config
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'sessions' 
        AND column_name = 'context_config'
    ) THEN
        ALTER TABLE sessions 
        ADD COLUMN context_config JSONB DEFAULT NULL;
        
        COMMENT ON COLUMN sessions.context_config IS 'LLM context management configuration (separate from message storage)';
        
        RAISE NOTICE 'Added context_config column to sessions table';
    ELSE
        RAISE NOTICE 'context_config column already exists in sessions table';
    END IF;
END $$;

-- For messages table: add agent_steps
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'messages' 
        AND column_name = 'agent_steps'
    ) THEN
        ALTER TABLE messages 
        ADD COLUMN agent_steps JSONB DEFAULT NULL;
        
        COMMENT ON COLUMN messages.agent_steps IS 'Agent execution steps (reasoning process and tool calls)';
        
        RAISE NOTICE 'Added agent_steps column to messages table';
    ELSE
        RAISE NOTICE 'agent_steps column already exists in messages table';
    END IF;
END $$;

-- Add GIN indexes for JSON fields
DO $$
BEGIN
    -- For tenants.agent_config
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'tenants' 
        AND indexname = 'idx_tenants_agent_config'
    ) THEN
        IF EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_name = 'tenants' 
            AND column_name = 'agent_config'
            AND data_type = 'jsonb'
        ) THEN
            CREATE INDEX idx_tenants_agent_config ON tenants USING GIN (agent_config);
            RAISE NOTICE 'Created index idx_tenants_agent_config';
        ELSE
            RAISE NOTICE 'Skipped index creation for tenants.agent_config (not JSONB type)';
        END IF;
    ELSE
        RAISE NOTICE 'Index idx_tenants_agent_config already exists';
    END IF;
    
    -- For sessions.agent_config
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'sessions' 
        AND indexname = 'idx_sessions_agent_config'
    ) THEN
        CREATE INDEX idx_sessions_agent_config ON sessions USING GIN (agent_config);
        RAISE NOTICE 'Created index idx_sessions_agent_config';
    ELSE
        RAISE NOTICE 'Index idx_sessions_agent_config already exists';
    END IF;
    
    -- For sessions.context_config
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'sessions' 
        AND indexname = 'idx_sessions_context_config'
    ) THEN
        CREATE INDEX idx_sessions_context_config ON sessions USING GIN (context_config);
        RAISE NOTICE 'Created index idx_sessions_context_config';
    ELSE
        RAISE NOTICE 'Index idx_sessions_context_config already exists';
    END IF;
    
    -- For messages.agent_steps
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'messages' 
        AND indexname = 'idx_messages_agent_steps'
    ) THEN
        CREATE INDEX idx_messages_agent_steps ON messages USING GIN (agent_steps);
        RAISE NOTICE 'Created index idx_messages_agent_steps';
    ELSE
        RAISE NOTICE 'Index idx_messages_agent_steps already exists';
    END IF;
END $$;

COMMIT;

BEGIN;

WITH referenced_models AS (
    SELECT embedding_model_id AS model_id FROM knowledge_bases WHERE deleted_at IS NULL AND embedding_model_id != ''
    UNION
    SELECT summary_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND summary_model_id != ''
    UNION
    SELECT rerank_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND rerank_model_id != ''
    UNION
    SELECT vlm_config ->> 'model_id'
    FROM knowledge_bases
    WHERE deleted_at IS NULL
      AND vlm_config ->> 'model_id' IS NOT NULL
      AND vlm_config ->> 'model_id' != ''
    UNION
    SELECT embedding_model_id FROM knowledges WHERE deleted_at IS NULL AND embedding_model_id IS NOT NULL AND embedding_model_id != ''
)
UPDATE models m
SET deleted_at = CURRENT_TIMESTAMP
WHERE m.deleted_at IS NULL
  AND m.is_default = FALSE
  AND m.tenant_id != 0
  AND m.id NOT IN (SELECT model_id FROM referenced_models);

COMMIT;

BEGIN;

-- Create mcp_services table
CREATE TABLE IF NOT EXISTS mcp_services (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    transport_type VARCHAR(50) NOT NULL,
    url VARCHAR(512) NOT NULL,
    headers JSONB,
    auth_config JSONB,
    advanced_config JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_mcp_services_tenant_id ON mcp_services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mcp_services_enabled ON mcp_services(enabled);
CREATE INDEX IF NOT EXISTS idx_mcp_services_deleted_at ON mcp_services(deleted_at);

-- Add comment to table
COMMENT ON TABLE mcp_services IS 'MCP service configurations';

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_mcp_services_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_mcp_services_updated_at
    BEFORE UPDATE ON mcp_services
    FOR EACH ROW
    EXECUTE FUNCTION update_mcp_services_updated_at();

COMMIT;

BEGIN;

-- Add web_search_config column to tenants table
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'tenants' AND column_name = 'web_search_config'
    ) THEN
        ALTER TABLE tenants 
        ADD COLUMN web_search_config JSONB DEFAULT NULL;
    END IF;
END $$;

-- Add or update comment
COMMENT ON COLUMN tenants.web_search_config IS 'Web search configuration for the tenant';

COMMIT;

BEGIN;

-- Add stdio_config and env_vars columns
ALTER TABLE mcp_services 
ADD COLUMN IF NOT EXISTS stdio_config JSONB,
ADD COLUMN IF NOT EXISTS env_vars JSONB;

-- Make url column optional
ALTER TABLE mcp_services 
ALTER COLUMN url DROP NOT NULL;

-- Add check constraint
ALTER TABLE mcp_services
ADD CONSTRAINT chk_mcp_transport_config CHECK (
    (transport_type = 'stdio' AND stdio_config IS NOT NULL) OR
    (transport_type != 'stdio' AND url IS NOT NULL)
);

COMMIT;

BEGIN;

-- Add is_builtin column to models table
ALTER TABLE models 
ADD COLUMN IF NOT EXISTS is_builtin BOOLEAN NOT NULL DEFAULT false;

-- Add index for is_builtin field
CREATE INDEX IF NOT EXISTS idx_models_is_builtin ON models(is_builtin);

COMMIT;

BEGIN;

ALTER TABLE knowledge_bases
  ADD COLUMN IF NOT EXISTS type VARCHAR(32) NOT NULL DEFAULT 'document',
  ADD COLUMN IF NOT EXISTS faq_config JSONB;

UPDATE knowledge_bases
SET type = 'document'
WHERE type IS NULL OR type = '';

ALTER TABLE chunks
  ADD COLUMN IF NOT EXISTS metadata JSONB;

COMMIT;

BEGIN;

-- Tag table (per knowledge base)
CREATE TABLE IF NOT EXISTS knowledge_tags (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(128) NOT NULL,
    color VARCHAR(32),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_kb_name
    ON knowledge_tags(tenant_id, knowledge_base_id, name);

CREATE INDEX IF NOT EXISTS idx_knowledge_tags_kb
    ON knowledge_tags(tenant_id, knowledge_base_id);

-- Tag reference on knowledges
ALTER TABLE knowledges
    ADD COLUMN IF NOT EXISTS tag_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_knowledges_tag
    ON knowledges(tag_id);

-- Tag reference on chunks
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS tag_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_chunks_tag
    ON chunks(tag_id);

COMMIT;

BEGIN;

-- Add is_enabled column to embeddings table
ALTER TABLE embeddings
    ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN DEFAULT TRUE;

-- Create index for is_enabled column
CREATE INDEX IF NOT EXISTS idx_embeddings_is_enabled
    ON embeddings(is_enabled);

COMMIT;

BEGIN;

-- Create index for knowledge_base_id
CREATE INDEX IF NOT EXISTS idx_embeddings_knowledge_base_id
    ON embeddings(knowledge_base_id);

COMMIT;

REINDEX INDEX embeddings_search_idx;

BEGIN;

-- Add status field to track chunk processing state
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS status INT NOT NULL DEFAULT 0;

-- Add content_hash field for quick content matching
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS content_hash VARCHAR(64);

-- Create index on content_hash
CREATE INDEX IF NOT EXISTS idx_chunks_content_hash
    ON chunks(content_hash);

COMMIT;

BEGIN;

WITH tenant_source AS (
    SELECT 
        kb.embedding_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.embedding_model_id IS NOT NULL
      AND kb.embedding_model_id <> ''

    UNION

    SELECT 
        kb.summary_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.summary_model_id IS NOT NULL
      AND kb.summary_model_id <> ''

    UNION

    SELECT
        kb.rerank_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.rerank_model_id IS NOT NULL
      AND kb.rerank_model_id <> ''
)
UPDATE models m
SET tenant_id = ts.tenant_id
FROM tenant_source ts
WHERE m.id = ts.model_id
  AND m.tenant_id = 0;

COMMIT;

BEGIN;

ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS rerank_model_id;

COMMIT;

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS can_access_all_tenants BOOLEAN NOT NULL DEFAULT FALSE;

COMMIT;

ALTER TABLE knowledge_bases 
ADD COLUMN IF NOT EXISTS question_generation_config JSONB NULL ;

ALTER TABLE knowledges ADD COLUMN IF NOT EXISTS summary_status VARCHAR(32) DEFAULT 'none';

-- Add index for efficient querying
CREATE INDEX IF NOT EXISTS idx_knowledges_summary_status ON knowledges(summary_status);
