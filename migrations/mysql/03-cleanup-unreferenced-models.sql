-- ============================================================================
-- Migration: Cleanup Unreferenced Models
-- Description: Remove models that are not referenced by any knowledge base
-- Created: 2025-11-03
-- ============================================================================

-- This script removes models from the models table that meet ALL of these conditions:
-- 1. Not referenced by any knowledge_bases (embedding_model_id, summary_model_id, rerank_model_id, vlm_config.model_id)
-- 2. Not referenced by any knowledges (embedding_model_id)
-- 3. Not a default model (is_default = false)
-- 4. Not a system model (tenant_id != 0)
-- 5. Not soft-deleted (deleted_at IS NULL)

-- WARNING: This operation is irreversible. Make sure to backup your database before running.
-- Recommended: Run the SELECT query first to review which models will be deleted.

-- ============================================================================
-- Step 1: Review models that will be deleted (DRY RUN)
-- ============================================================================
SELECT 
    m.id,
    m.tenant_id,
    m.name,
    m.type,
    m.source,
    m.is_default,
    m.status,
    m.created_at
FROM models m
WHERE m.deleted_at IS NULL
  AND m.is_default = FALSE
  AND m.tenant_id != 0
  AND m.id NOT IN (
      -- Models referenced by knowledge_bases
      SELECT DISTINCT embedding_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND embedding_model_id != ''
      UNION
      SELECT DISTINCT summary_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND summary_model_id != ''
      UNION
      SELECT DISTINCT rerank_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND rerank_model_id != ''
      UNION
      SELECT DISTINCT JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) AS vlm_model_id
      FROM knowledge_bases
      WHERE deleted_at IS NULL
        AND JSON_EXTRACT(vlm_config, '$.model_id') IS NOT NULL
        AND JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) != ''
      UNION
      -- Models referenced by knowledges
      SELECT DISTINCT embedding_model_id FROM knowledges WHERE deleted_at IS NULL AND embedding_model_id IS NOT NULL AND embedding_model_id != ''
  )
ORDER BY m.created_at DESC;

-- ============================================================================
-- Step 2: Soft delete unreferenced models (set deleted_at timestamp)
-- ============================================================================
-- Uncomment the following line to perform soft delete:
-- UPDATE models m
-- SET deleted_at = CURRENT_TIMESTAMP
-- WHERE m.deleted_at IS NULL
--   AND m.is_default = FALSE
--   AND m.tenant_id != 0
--   AND m.id NOT IN (
--       SELECT DISTINCT embedding_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND embedding_model_id != ''
--       UNION
--       SELECT DISTINCT summary_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND summary_model_id != ''
--       UNION
--       SELECT DISTINCT rerank_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND rerank_model_id != ''
--       UNION
--       SELECT DISTINCT JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) AS vlm_model_id
--       FROM knowledge_bases
--       WHERE deleted_at IS NULL
--         AND JSON_EXTRACT(vlm_config, '$.model_id') IS NOT NULL
--         AND JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) != ''
--       UNION
--       SELECT DISTINCT embedding_model_id FROM knowledges WHERE deleted_at IS NULL AND embedding_model_id IS NOT NULL AND embedding_model_id != ''
--   );

-- ============================================================================
-- Step 3: Hard delete unreferenced models (PERMANENT DELETION)
-- ============================================================================
-- WARNING: This is permanent and cannot be undone!
-- Only use this if you're sure you want to permanently remove the records.
-- Uncomment the following line to perform hard delete:
-- DELETE FROM models
-- WHERE deleted_at IS NULL
--   AND is_default = FALSE
--   AND tenant_id != 0
--   AND id NOT IN (
--       SELECT DISTINCT embedding_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND embedding_model_id != ''
--       UNION
--       SELECT DISTINCT summary_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND summary_model_id != ''
--       UNION
--       SELECT DISTINCT rerank_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND rerank_model_id != ''
--       UNION
--       SELECT DISTINCT JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) AS vlm_model_id
--       FROM knowledge_bases
--       WHERE deleted_at IS NULL
--         AND JSON_EXTRACT(vlm_config, '$.model_id') IS NOT NULL
--         AND JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) != ''
--       UNION
--       SELECT DISTINCT embedding_model_id FROM knowledges WHERE deleted_at IS NULL AND embedding_model_id IS NOT NULL AND embedding_model_id != ''
--   );

-- ============================================================================
-- Additional Queries for Statistics
-- ============================================================================

-- Count total models by type
SELECT 
    type,
    COUNT(*) as total_count,
    SUM(CASE WHEN is_default = TRUE THEN 1 ELSE 0 END) as default_count,
    SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END) as active_count
FROM models
GROUP BY type;

-- Count models referenced by knowledge bases
SELECT 
    'embedding_model' as model_type,
    COUNT(DISTINCT embedding_model_id) as referenced_count
FROM knowledge_bases
WHERE deleted_at IS NULL AND embedding_model_id != ''
UNION ALL
SELECT 
    'summary_model' as model_type,
    COUNT(DISTINCT summary_model_id) as referenced_count
FROM knowledge_bases
WHERE deleted_at IS NULL AND summary_model_id != ''
UNION ALL
SELECT 
    'rerank_model' as model_type,
    COUNT(DISTINCT rerank_model_id) as referenced_count
FROM knowledge_bases
WHERE deleted_at IS NULL AND rerank_model_id != ''
UNION ALL
SELECT 
    'vlm_model' as model_type,
    COUNT(DISTINCT JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id'))) as referenced_count
FROM knowledge_bases
WHERE deleted_at IS NULL
  AND JSON_EXTRACT(vlm_config, '$.model_id') IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(vlm_config, '$.model_id')) != '';

-- ============================================================================
-- Rollback Strategy
-- ============================================================================
-- If you performed a soft delete and need to rollback:
-- UPDATE models 
-- SET deleted_at = NULL 
-- WHERE deleted_at > 'YOUR_DELETION_TIMESTAMP';
-- ============================================================================

