-- Remove summary_status column from knowledges table

DROP INDEX IF EXISTS idx_knowledges_summary_status;
ALTER TABLE knowledges DROP COLUMN IF EXISTS summary_status;
