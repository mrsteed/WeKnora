-- 000009_add_kb_tags.up.sql
-- Add knowledge base scoped tags and tag_id fields

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


