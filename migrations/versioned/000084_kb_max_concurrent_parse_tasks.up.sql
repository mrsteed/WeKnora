ALTER TABLE knowledge_bases
    ADD COLUMN IF NOT EXISTS max_concurrent_parse_tasks INTEGER NOT NULL DEFAULT 5;
