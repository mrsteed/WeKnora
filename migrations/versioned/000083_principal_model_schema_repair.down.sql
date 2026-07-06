-- Rollback for 000083_principal_model_schema_repair.
--
-- This migration is a schema repair pass for drifted environments. Reverting
-- it would mean dropping compatibility columns or narrowing varchar widths
-- again, which would risk data loss and reintroduce runtime failures.
-- Keep the repair in place; this file exists only to satisfy the up/down pair
-- required by golang-migrate.
DO $$ BEGIN RAISE NOTICE '[Migration 000083] No-op rollback (schema repair is one-way)'; END $$;