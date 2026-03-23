CREATE TABLE plugins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    plugin_type TEXT NOT NULL CHECK (plugin_type IN ('native', 'grpc')),
    config      JSONB NOT NULL DEFAULT '{}',
    target      TEXT NOT NULL DEFAULT 'global' CHECK (target IN ('global', 'route', 'consumer')),
    grpc_address TEXT,
    ordering    INTEGER NOT NULL DEFAULT 0,
    is_enabled  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_plugins_enabled ON plugins (is_enabled) WHERE is_enabled = true;
CREATE INDEX idx_plugins_ordering ON plugins (ordering);

-- Trigger NOTIFY for hot reload
CREATE TRIGGER plugins_notify
    AFTER INSERT OR UPDATE OR DELETE ON plugins
    FOR EACH ROW EXECUTE FUNCTION notify_config_change();
