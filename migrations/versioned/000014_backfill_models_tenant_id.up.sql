-- 14-backfill-models-tenant-id.up.sql
-- Backfill tenant_id in models table using knowledges records that share the same id

BEGIN;

WITH tenant_source AS (
    -- 从 knowledge_bases 中取出各模型 ID（embedding / summary）及其 tenant_id
    SELECT 
        kb.embedding_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.embedding_model_id IS NOT NULL
      AND kb.embedding_model_id <> ''

    UNION

    SELECT 
        kb.summary_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.summary_model_id IS NOT NULL
      AND kb.summary_model_id <> ''

    UNION

    -- rerank 模型
    SELECT
        kb.rerank_model_id AS model_id,
        kb.tenant_id
    FROM knowledge_bases kb
    WHERE kb.tenant_id IS NOT NULL
      AND kb.rerank_model_id IS NOT NULL
      AND kb.rerank_model_id <> ''
)
UPDATE models m
SET tenant_id = ts.tenant_id
FROM tenant_source ts
WHERE m.id = ts.model_id
  AND m.tenant_id = 0;

COMMIT;


