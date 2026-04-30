ALTER TABLE messages ADD COLUMN completion_status VARCHAR(32);
ALTER TABLE messages ADD COLUMN finish_reason VARCHAR(32);
ALTER TABLE messages ADD COLUMN failure_reason TEXT;

UPDATE messages
SET completion_status = CASE
    WHEN role = 'assistant' AND is_completed = 1 THEN 'completed'
    WHEN role = 'assistant' AND is_completed = 0 THEN 'pending'
    ELSE 'completed'
END
WHERE completion_status IS NULL;