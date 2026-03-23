-- 003_create_api_keys.up.sql
-- Creates the api_keys table for API key authentication with lifecycle management.

CREATE TABLE IF NOT EXISTS api_keys (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_id         UUID NOT NULL REFERENCES consumers(id) ON DELETE CASCADE,
    key_hash            TEXT NOT NULL,
    key_prefix          TEXT NOT NULL,
    is_active           BOOLEAN NOT NULL DEFAULT true,
    deprecated_at       TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ,
    rate_limit_override JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for key lookup by prefix during authentication.
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys (key_prefix) WHERE is_active = true;

-- Index for listing keys by consumer.
CREATE INDEX IF NOT EXISTS idx_api_keys_consumer ON api_keys (consumer_id);

-- Index for filtering active keys.
CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys (is_active) WHERE is_active = true;

-- NOTIFY on api_keys changes so data plane nodes can invalidate caches.
CREATE TRIGGER trg_api_keys_notify
    AFTER INSERT OR UPDATE OR DELETE ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION notify_config_change();
