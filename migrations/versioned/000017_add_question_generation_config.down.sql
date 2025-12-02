-- Remove question_generation_config column from knowledge_bases table

ALTER TABLE knowledge_bases 
DROP COLUMN IF EXISTS question_generation_config;
