-- Migration: 000083_principal_model_schema_repair
-- Description: Re-apply the principal-model postconditions from 000064 for
-- environments whose schema metadata advanced while one or more columns or
-- widths were still missing.

DO $$ BEGIN RAISE NOTICE '[Migration 000083] Repairing principal-model schema drift'; END $$;

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS api_principal_config JSONB;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = current_schema()
          AND table_name = 'mcp_oauth_tokens'
    ) THEN
        ALTER TABLE mcp_oauth_tokens
            ADD COLUMN IF NOT EXISTS principal_type VARCHAR(32),
            ADD COLUMN IF NOT EXISTS principal_id VARCHAR(512);

        IF EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'mcp_oauth_tokens'
              AND column_name = 'user_id'
              AND character_maximum_length IS NOT NULL
              AND character_maximum_length < 512
        ) THEN
            ALTER TABLE mcp_oauth_tokens
                ALTER COLUMN user_id TYPE VARCHAR(512);
        END IF;

        IF EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'mcp_oauth_tokens'
              AND column_name = 'principal_type'
              AND character_maximum_length IS NOT NULL
              AND character_maximum_length < 32
        ) THEN
            ALTER TABLE mcp_oauth_tokens
                ALTER COLUMN principal_type TYPE VARCHAR(32);
        END IF;

        IF EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'mcp_oauth_tokens'
              AND column_name = 'principal_id'
              AND character_maximum_length IS NOT NULL
              AND character_maximum_length < 512
        ) THEN
            ALTER TABLE mcp_oauth_tokens
                ALTER COLUMN principal_id TYPE VARCHAR(512);
        END IF;

        UPDATE mcp_oauth_tokens
        SET principal_type = CASE
                WHEN principal_type IS NULL OR principal_type = '' THEN 'web_user'
                ELSE principal_type
            END,
            principal_id = CASE
                WHEN principal_id IS NULL OR principal_id = '' THEN user_id
                ELSE principal_id
            END
        WHERE principal_type IS NULL OR principal_type = '' OR principal_id IS NULL OR principal_id = '';

        IF EXISTS (
            SELECT 1
            FROM mcp_oauth_tokens
            WHERE principal_type IS NULL OR principal_type = '' OR principal_id IS NULL OR principal_id = ''
        ) THEN
            RAISE EXCEPTION '[Migration 000083] mcp_oauth_tokens still contains rows without principal identity after repair';
        END IF;

        ALTER TABLE mcp_oauth_tokens
            ALTER COLUMN principal_type SET NOT NULL,
            ALTER COLUMN principal_id SET NOT NULL;

        DROP INDEX IF EXISTS idx_mcp_oauth_tokens_tenant_user_svc;

        CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_tenant_principal_svc
            ON mcp_oauth_tokens(tenant_id, principal_type, principal_id, service_id);

        CREATE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_principal
            ON mcp_oauth_tokens(principal_type, principal_id);
    ELSE
        RAISE NOTICE '[Migration 000083] mcp_oauth_tokens table missing, skipping OAuth principal repair';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'sessions'
          AND column_name = 'user_id'
          AND character_maximum_length IS NOT NULL
          AND character_maximum_length < 512
    ) THEN
        ALTER TABLE sessions
            ALTER COLUMN user_id TYPE VARCHAR(512);
    END IF;
END $$;

DO $$ BEGIN RAISE NOTICE '[Migration 000083] principal-model schema repair complete'; END $$;