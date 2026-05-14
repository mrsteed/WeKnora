ALTER TABLE chat_document_generation_runs
    DROP COLUMN IF EXISTS runtime_feedback_json,
    DROP COLUMN IF EXISTS budget_json;