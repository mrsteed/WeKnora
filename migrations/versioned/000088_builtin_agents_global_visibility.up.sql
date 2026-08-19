-- Migration: 000088_builtin_agents_global_visibility
-- Description: Repair persisted built-in agent config override rows that were
--              created with the GORM column default and ended up as
--              visibility='private' with an empty created_by (no owner).
--
--              Under the private rule, a private resource is readable only by
--              its creator (CreatedBy must be non-empty and equal to the
--              requesting user). A "private owned by nobody" row therefore
--              denies read access to every non-privileged tenant member —
--              per-id agent detail, copy, and suggested-questions all return
--              1002, while list/detail-free listings still show the agent
--              (ListAgents appends built-ins unconditionally). Net effect:
--              built-in agents were visible-but-unusable for regular users.
--
--              Built-in agents are tenant-wide platform resources and must be
--              readable by everyone in the tenant. Set them to 'global'.
--
--              This row-level repair complements the code change in
--              canReadResource/canAccessAgent (built-in IDs bypass the
--              private-visibility rule) and in updateBuiltinAgent (new
--              override rows are now persisted with visibility='global').

UPDATE custom_agents
SET visibility = 'global'
WHERE is_builtin
  AND deleted_at IS NULL
  AND COALESCE(visibility, '') <> 'global';

DO $$ BEGIN RAISE NOTICE '[Migration 000088 UP] built-in agents set to global visibility'; END $$;
