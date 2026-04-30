-- Add completion state columns to messages for P1 protocol/state-machine alignment.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS completion_status VARCHAR(32);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS finish_reason VARCHAR(32);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS failure_reason TEXT;

UPDATE messages
SET completion_status = CASE
    WHEN role = 'assistant' AND is_completed THEN 'completed'
    WHEN role = 'assistant' AND NOT is_completed THEN 'pending'
    ELSE 'completed'
END
WHERE completion_status IS NULL;