-- 010_add_org_id_and_rls.up.sql
-- Adds organization_id to all existing tables, backfills with default org,
-- sets NOT NULL constraints, updates unique constraints, and enables RLS.

-- =============================================================================
-- 1. Add nullable organization_id to all tables
-- =============================================================================

ALTER TABLE routes ADD COLUMN organization_id UUID;
ALTER TABLE backends ADD COLUMN organization_id UUID;
ALTER TABLE consumers ADD COLUMN organization_id UUID;
ALTER TABLE api_keys ADD COLUMN organization_id UUID;
ALTER TABLE rate_limit_counters ADD COLUMN organization_id UUID;
ALTER TABLE ai_models ADD COLUMN organization_id UUID;
ALTER TABLE ai_usage_log ADD COLUMN organization_id UUID;
ALTER TABLE token_budgets ADD COLUMN organization_id UUID;
ALTER TABLE plugins ADD COLUMN organization_id UUID;
ALTER TABLE relationships ADD COLUMN organization_id UUID;

-- =============================================================================
-- 2. Create default organization and backfill all existing rows
-- =============================================================================

INSERT INTO organizations (id, name, slug, billing_email)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Organization', 'default', 'admin@flexgateway.io')
ON CONFLICT (slug) DO NOTHING;

UPDATE routes SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE backends SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE consumers SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE api_keys SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE rate_limit_counters SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE ai_models SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE ai_usage_log SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE token_budgets SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE plugins SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;
UPDATE relationships SET organization_id = '00000000-0000-0000-0000-000000000001' WHERE organization_id IS NULL;

-- =============================================================================
-- 3. Set NOT NULL after backfill
-- =============================================================================

ALTER TABLE routes ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE backends ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE consumers ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE rate_limit_counters ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE ai_models ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE ai_usage_log ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE token_budgets ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE plugins ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE relationships ALTER COLUMN organization_id SET NOT NULL;

-- =============================================================================
-- 4. Add FK constraints
-- =============================================================================

ALTER TABLE routes ADD CONSTRAINT fk_routes_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE backends ADD CONSTRAINT fk_backends_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE consumers ADD CONSTRAINT fk_consumers_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE api_keys ADD CONSTRAINT fk_api_keys_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE ai_models ADD CONSTRAINT fk_ai_models_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE ai_usage_log ADD CONSTRAINT fk_ai_usage_log_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE token_budgets ADD CONSTRAINT fk_token_budgets_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE plugins ADD CONSTRAINT fk_plugins_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
ALTER TABLE relationships ADD CONSTRAINT fk_relationships_org FOREIGN KEY (organization_id) REFERENCES organizations(id);
-- rate_limit_counters is UNLOGGED; FK not added to avoid write-ahead overhead.

-- =============================================================================
-- 5. Update unique constraints from (name) to (organization_id, name)
-- =============================================================================

-- routes: (name) -> (organization_id, name)
ALTER TABLE routes DROP CONSTRAINT routes_name_key;
ALTER TABLE routes ADD CONSTRAINT routes_org_name_key UNIQUE (organization_id, name);

-- backends: (name) -> (organization_id, name)
ALTER TABLE backends DROP CONSTRAINT backends_name_key;
ALTER TABLE backends ADD CONSTRAINT backends_org_name_key UNIQUE (organization_id, name);

-- consumers: (name) -> (organization_id, name)
ALTER TABLE consumers DROP CONSTRAINT consumers_name_key;
ALTER TABLE consumers ADD CONSTRAINT consumers_org_name_key UNIQUE (organization_id, name);

-- ai_models: (provider, model_name) -> (organization_id, provider, model_name)
ALTER TABLE ai_models DROP CONSTRAINT ai_models_provider_model_name_key;
ALTER TABLE ai_models ADD CONSTRAINT ai_models_org_provider_model_key UNIQUE (organization_id, provider, model_name);

-- plugins: (name) -> (organization_id, name)
ALTER TABLE plugins DROP CONSTRAINT plugins_name_key;
ALTER TABLE plugins ADD CONSTRAINT plugins_org_name_key UNIQUE (organization_id, name);

-- =============================================================================
-- 6. Create indexes on organization_id for all tables
-- =============================================================================

CREATE INDEX idx_routes_org ON routes(organization_id);
CREATE INDEX idx_backends_org ON backends(organization_id);
CREATE INDEX idx_consumers_org ON consumers(organization_id);
CREATE INDEX idx_api_keys_org ON api_keys(organization_id);
CREATE INDEX idx_rate_limit_counters_org ON rate_limit_counters(organization_id);
CREATE INDEX idx_ai_models_org ON ai_models(organization_id);
CREATE INDEX idx_ai_usage_log_org ON ai_usage_log(organization_id);
CREATE INDEX idx_token_budgets_org ON token_budgets(organization_id);
CREATE INDEX idx_plugins_org ON plugins(organization_id);
CREATE INDEX idx_relationships_org ON relationships(organization_id);

-- =============================================================================
-- 7. Enable RLS on all tenant-scoped tables
-- =============================================================================

ALTER TABLE routes ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_routes ON routes
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE backends ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_backends ON backends
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE consumers ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_consumers ON consumers
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_api_keys ON api_keys
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE rate_limit_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_rate_limit_counters ON rate_limit_counters
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE ai_models ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_ai_models ON ai_models
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE ai_usage_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_ai_usage_log ON ai_usage_log
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE token_budgets ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_token_budgets ON token_budgets
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE plugins ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_plugins ON plugins
    USING (organization_id = current_setting('app.current_org')::uuid);

ALTER TABLE relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_relationships ON relationships
    USING (organization_id = current_setting('app.current_org')::uuid);

-- =============================================================================
-- 8. Update notify_config_change to include organization_id in payload
-- =============================================================================

CREATE OR REPLACE FUNCTION notify_config_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('config_changes', json_build_object(
        'table', TG_TABLE_NAME,
        'action', TG_OP,
        'id', COALESCE(NEW.id, OLD.id)::text,
        'organization_id', COALESCE(NEW.organization_id, OLD.organization_id)::text
    )::text);
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
