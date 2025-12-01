-- 000017_cleanup_unreferenced_models.up.sql
-- Cleanup unreferenced models by soft-deleting them (set deleted_at)
-- Ported from migrations/paradedb/03-cleanup-unreferenced-models.sql
BEGIN;
WITH referenced_models AS (
    SELECT embedding_model_id AS model_id FROM knowledge_bases WHERE deleted_at IS NULL AND embedding_model_id != ''
    UNION
    SELECT summary_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND summary_model_id != ''
    UNION
    SELECT rerank_model_id FROM knowledge_bases WHERE deleted_at IS NULL AND rerank_model_id != ''
    UNION
    SELECT vlm_config ->> 'model_id'
    FROM knowledge_bases
    WHERE deleted_at IS NULL
      AND vlm_config ->> 'model_id' IS NOT NULL
      AND vlm_config ->> 'model_id' != ''
    UNION
    SELECT embedding_model_id FROM knowledges WHERE deleted_at IS NULL AND embedding_model_id IS NOT NULL AND embedding_model_id != ''
)
UPDATE models m
SET deleted_at = CURRENT_TIMESTAMP
WHERE m.deleted_at IS NULL
  AND m.is_default = FALSE
  AND m.tenant_id != 0
  AND m.id NOT IN (SELECT model_id FROM referenced_models);
COMMIT;



