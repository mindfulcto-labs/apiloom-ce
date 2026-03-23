-- 001_create_config_tables.up.sql
-- Creates the core config tables for FlexGateway: routes, backends, and relationships.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Routes table: stores route configurations with JSONB config and metadata.
CREATE TABLE IF NOT EXISTS routes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL UNIQUE,
    version    INTEGER NOT NULL DEFAULT 1,
    config     JSONB NOT NULL,
    metadata   JSONB DEFAULT '{}',
    checksum   VARCHAR(64) NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Backends table: stores backend/upstream configurations.
CREATE TABLE IF NOT EXISTS backends (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL UNIQUE,
    config     JSONB NOT NULL,
    metadata   JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Relationships table: directed edges between config entities.
-- Prepares for Apache AGE graph migration in Phase 2.
CREATE TABLE IF NOT EXISTS relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type   VARCHAR(64) NOT NULL,
    source_id     UUID NOT NULL,
    relationship  VARCHAR(64) NOT NULL,
    target_type   VARCHAR(64) NOT NULL,
    target_id     UUID NOT NULL,
    metadata      JSONB DEFAULT '{}'
);

-- Indexes for common lookups.
CREATE INDEX IF NOT EXISTS idx_routes_name ON routes (name);
CREATE INDEX IF NOT EXISTS idx_routes_active ON routes (is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_backends_name ON backends (name);
CREATE INDEX IF NOT EXISTS idx_relationships_source ON relationships (source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships (target_type, target_id);

-- Auto-update updated_at on row modification.
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_routes_updated_at
    BEFORE UPDATE ON routes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_backends_updated_at
    BEFORE UPDATE ON backends
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- NOTIFY on route config changes so data plane nodes can hot-reload.
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

CREATE TRIGGER trg_routes_notify
    AFTER INSERT OR UPDATE OR DELETE ON routes
    FOR EACH ROW
    EXECUTE FUNCTION notify_config_change();

CREATE TRIGGER trg_backends_notify
    AFTER INSERT OR UPDATE OR DELETE ON backends
    FOR EACH ROW
    EXECUTE FUNCTION notify_config_change();
