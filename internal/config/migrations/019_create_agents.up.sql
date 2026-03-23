-- Create agents table for AI agent identity and governance.
CREATE TABLE IF NOT EXISTS agents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_id    UUID NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    trust_level VARCHAR(20) NOT NULL DEFAULT 'untrusted'
        CHECK (trust_level IN ('untrusted', 'basic', 'elevated', 'admin')),
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'revoked')),
    api_key_hash TEXT NOT NULL DEFAULT '',
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT agents_org_name_unique UNIQUE (org_id, name)
);

-- Index on status for filtered listing.
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents (status);

-- Index on org_id for org-scoped queries.
CREATE INDEX IF NOT EXISTS idx_agents_org_id ON agents (org_id);

-- GIN index on capabilities for contains queries.
CREATE INDEX IF NOT EXISTS idx_agents_capabilities ON agents USING GIN (capabilities);
