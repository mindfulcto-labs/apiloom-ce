-- 011_force_rls.up.sql
-- Create an application role that is subject to RLS, and grant it access.
-- Superusers bypass RLS even with FORCE ROW LEVEL SECURITY, so the
-- application MUST connect as this non-superuser role for RLS to work.

-- Create the application role if it doesn't exist.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'flexgateway_app') THEN
        CREATE ROLE flexgateway_app WITH LOGIN PASSWORD 'flexgateway_app';
    END IF;
END
$$;

-- Grant usage on the public schema and all tables.
GRANT USAGE ON SCHEMA public TO flexgateway_app;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO flexgateway_app;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO flexgateway_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO flexgateway_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO flexgateway_app;

-- Force RLS on all tables (applies to table owner, but superuser still bypasses).
ALTER TABLE routes FORCE ROW LEVEL SECURITY;
ALTER TABLE backends FORCE ROW LEVEL SECURITY;
ALTER TABLE consumers FORCE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;
ALTER TABLE rate_limit_counters FORCE ROW LEVEL SECURITY;
ALTER TABLE ai_models FORCE ROW LEVEL SECURITY;
ALTER TABLE ai_usage_log FORCE ROW LEVEL SECURITY;
ALTER TABLE token_budgets FORCE ROW LEVEL SECURITY;
ALTER TABLE plugins FORCE ROW LEVEL SECURITY;
ALTER TABLE relationships FORCE ROW LEVEL SECURITY;
