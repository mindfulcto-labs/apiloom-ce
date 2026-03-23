-- 002_create_consumers.up.sql
-- Creates the consumers table for API consumer identity management.

CREATE TABLE IF NOT EXISTS consumers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    metadata   JSONB DEFAULT '{}',
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for filtering active consumers.
CREATE INDEX IF NOT EXISTS idx_consumers_active ON consumers (is_active) WHERE is_active = true;

-- Auto-update updated_at on row modification.
CREATE TRIGGER trg_consumers_updated_at
    BEFORE UPDATE ON consumers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();
