-- 09_add_kb_tags.sql
-- Add knowledge base scoped tags and tag_id fields for knowledge and chunks
BEGIN;

-- Tag table (per knowledge base)
CREATE TABLE IF NOT EXISTS knowledge_tags (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(128) NOT NULL,
    color VARCHAR(32),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_kb_name
    ON knowledge_tags(tenant_id, knowledge_base_id, name);

CREATE INDEX IF NOT EXISTS idx_knowledge_tags_kb
    ON knowledge_tags(tenant_id, knowledge_base_id);

-- Tag reference on knowledges (document-type knowledge)
ALTER TABLE knowledges
    ADD COLUMN IF NOT EXISTS tag_id VARCHAR(36) NULL;

CREATE INDEX IF NOT EXISTS idx_knowledges_tag
    ON knowledges(tag_id);

-- Tag reference on chunks (FAQ entries)
ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS tag_id VARCHAR(36) NULL;

CREATE INDEX IF NOT EXISTS idx_chunks_tag
    ON chunks(tag_id);

COMMIT;


