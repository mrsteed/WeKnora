ALTER TABLE chat_document_generation_runs
    ADD COLUMN IF NOT EXISTS budget_json JSONB,
    ADD COLUMN IF NOT EXISTS runtime_feedback_json JSONB;