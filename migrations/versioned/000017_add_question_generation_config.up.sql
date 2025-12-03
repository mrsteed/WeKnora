-- Add question_generation_config column to knowledge_bases table
-- This column stores configuration for AI question generation feature
-- When enabled, the system generates questions for document chunks to improve recall

ALTER TABLE knowledge_bases 
ADD COLUMN IF NOT EXISTS question_generation_config JSONB NULL ;
