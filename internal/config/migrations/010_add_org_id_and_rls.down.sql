-- 010_add_org_id_and_rls.down.sql
-- Reverses: drop RLS policies, drop FK constraints, restore original unique constraints,
-- remove organization_id columns.

-- =============================================================================
-- 1. Restore notify_config_change without organization_id
-- =============================================================================

CREATE OR REPLACE FUNCTION notify_config_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('config_changes', json_build_object(
        'table', TG_TABLE_NAME,
        'action', TG_OP,
        'id', COALESCE(NEW.id, OLD.id)::text
    )::text);
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 2. Drop RLS policies and disable RLS
-- =============================================================================

DROP POLICY IF EXISTS org_isolation_routes ON routes;
ALTER TABLE routes DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_backends ON backends;
ALTER TABLE backends DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_consumers ON consumers;
ALTER TABLE consumers DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_api_keys ON api_keys;
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_rate_limit_counters ON rate_limit_counters;
ALTER TABLE rate_limit_counters DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_ai_models ON ai_models;
ALTER TABLE ai_models DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_ai_usage_log ON ai_usage_log;
ALTER TABLE ai_usage_log DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_token_budgets ON token_budgets;
ALTER TABLE token_budgets DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_plugins ON plugins;
ALTER TABLE plugins DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation_relationships ON relationships;
ALTER TABLE relationships DISABLE ROW LEVEL SECURITY;

-- =============================================================================
-- 3. Drop organization_id indexes
-- =============================================================================

DROP INDEX IF EXISTS idx_routes_org;
DROP INDEX IF EXISTS idx_backends_org;
DROP INDEX IF EXISTS idx_consumers_org;
DROP INDEX IF EXISTS idx_api_keys_org;
DROP INDEX IF EXISTS idx_rate_limit_counters_org;
DROP INDEX IF EXISTS idx_ai_models_org;
DROP INDEX IF EXISTS idx_ai_usage_log_org;
DROP INDEX IF EXISTS idx_token_budgets_org;
DROP INDEX IF EXISTS idx_plugins_org;
DROP INDEX IF EXISTS idx_relationships_org;

-- =============================================================================
-- 4. Restore original unique constraints
-- =============================================================================

ALTER TABLE routes DROP CONSTRAINT IF EXISTS routes_org_name_key;
ALTER TABLE routes ADD CONSTRAINT routes_name_key UNIQUE (name);

ALTER TABLE backends DROP CONSTRAINT IF EXISTS backends_org_name_key;
ALTER TABLE backends ADD CONSTRAINT backends_name_key UNIQUE (name);

ALTER TABLE consumers DROP CONSTRAINT IF EXISTS consumers_org_name_key;
ALTER TABLE consumers ADD CONSTRAINT consumers_name_key UNIQUE (name);

ALTER TABLE ai_models DROP CONSTRAINT IF EXISTS ai_models_org_provider_model_key;
ALTER TABLE ai_models ADD CONSTRAINT ai_models_provider_model_name_key UNIQUE (provider, model_name);

ALTER TABLE plugins DROP CONSTRAINT IF EXISTS plugins_org_name_key;
ALTER TABLE plugins ADD CONSTRAINT plugins_name_key UNIQUE (name);

-- =============================================================================
-- 5. Drop FK constraints
-- =============================================================================

ALTER TABLE routes DROP CONSTRAINT IF EXISTS fk_routes_org;
ALTER TABLE backends DROP CONSTRAINT IF EXISTS fk_backends_org;
ALTER TABLE consumers DROP CONSTRAINT IF EXISTS fk_consumers_org;
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS fk_api_keys_org;
ALTER TABLE ai_models DROP CONSTRAINT IF EXISTS fk_ai_models_org;
ALTER TABLE ai_usage_log DROP CONSTRAINT IF EXISTS fk_ai_usage_log_org;
ALTER TABLE token_budgets DROP CONSTRAINT IF EXISTS fk_token_budgets_org;
ALTER TABLE plugins DROP CONSTRAINT IF EXISTS fk_plugins_org;
ALTER TABLE relationships DROP CONSTRAINT IF EXISTS fk_relationships_org;

-- =============================================================================
-- 6. Drop organization_id columns
-- =============================================================================

ALTER TABLE routes DROP COLUMN IF EXISTS organization_id;
ALTER TABLE backends DROP COLUMN IF EXISTS organization_id;
ALTER TABLE consumers DROP COLUMN IF EXISTS organization_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS organization_id;
ALTER TABLE rate_limit_counters DROP COLUMN IF EXISTS organization_id;
ALTER TABLE ai_models DROP COLUMN IF EXISTS organization_id;
ALTER TABLE ai_usage_log DROP COLUMN IF EXISTS organization_id;
ALTER TABLE token_budgets DROP COLUMN IF EXISTS organization_id;
ALTER TABLE plugins DROP COLUMN IF EXISTS organization_id;
ALTER TABLE relationships DROP COLUMN IF EXISTS organization_id;
