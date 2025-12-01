-- 000016_add_agent_config_if_missing.up.sql
-- Incremental migration: add agent_config / context_config / agent_steps columns and indexes if missing
-- Ported from migrations/paradedb/02-add-agent-config-if-missing.sql

BEGIN;

-- ============================================
-- 1. 为 tenants 表添加 agent_config
-- ============================================

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
        -- 如果字段已存在但类型是 JSON，转换为 JSONB
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

-- ============================================
-- 2. 为 sessions 表添加 agent_config
-- ============================================

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

-- ============================================
-- 3. 为 sessions 表添加 context_config
-- ============================================

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

-- ============================================
-- 4. 为 messages 表添加 agent_steps
-- ============================================

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

-- ============================================
-- 5. 为 JSON 字段添加 GIN 索引（提高查询性能）
-- ============================================

DO $$
BEGIN
    -- 为 tenants.agent_config 添加索引（仅当字段类型为 JSONB 时）
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'tenants' 
        AND indexname = 'idx_tenants_agent_config'
    ) THEN
        -- 检查字段类型是否为 JSONB
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
    
    -- 为 sessions.agent_config 添加索引
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
    
    -- 为 sessions.context_config 添加索引
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
    
    -- 为 messages.agent_steps 添加索引
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



