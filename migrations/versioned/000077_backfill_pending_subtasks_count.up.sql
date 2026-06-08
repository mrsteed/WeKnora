ALTER TABLE knowledges
    ADD COLUMN IF NOT EXISTS pending_subtasks_count INT NOT NULL DEFAULT 0;