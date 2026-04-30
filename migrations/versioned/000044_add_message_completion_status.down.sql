ALTER TABLE messages DROP COLUMN IF EXISTS failure_reason;
ALTER TABLE messages DROP COLUMN IF EXISTS finish_reason;
ALTER TABLE messages DROP COLUMN IF EXISTS completion_status;