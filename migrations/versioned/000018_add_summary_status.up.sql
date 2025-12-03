-- Add summary_status column to knowledges table
-- This column tracks the status of async summary generation task
-- Values: none, pending, processing, completed, failed

ALTER TABLE knowledges ADD COLUMN IF NOT EXISTS summary_status VARCHAR(32) DEFAULT 'none';

-- Add index for efficient querying
CREATE INDEX IF NOT EXISTS idx_knowledges_summary_status ON knowledges(summary_status);
