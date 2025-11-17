-- Add conversation_config column to tenants table
-- This migration adds support for conversation configuration at tenant level
-- Used for normal mode conversation settings (prompt, context_template, temperature, max_tokens)

ALTER TABLE tenants 
ADD COLUMN conversation_config JSON DEFAULT NULL COMMENT 'Conversation configuration for normal mode sessions (prompt, context_template, temperature, max_tokens)';

