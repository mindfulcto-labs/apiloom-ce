CREATE TABLE config_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    version INTEGER NOT NULL,
    snapshot JSONB NOT NULL,
    author TEXT NOT NULL DEFAULT 'system',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, version)
);

CREATE INDEX idx_config_versions_org_created ON config_versions(organization_id, created_at DESC);

ALTER TABLE config_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_versions FORCE ROW LEVEL SECURITY;

CREATE POLICY config_versions_org_isolation ON config_versions
    USING (organization_id = current_setting('app.current_org')::uuid);
