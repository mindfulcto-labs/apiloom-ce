CREATE TABLE IF NOT EXISTS environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT DEFAULT '',
    is_production BOOLEAN DEFAULT false,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(organization_id, slug)
);

CREATE INDEX idx_environments_org ON environments (organization_id);

-- Seed default environments for the default org
INSERT INTO environments (organization_id, name, slug, is_production, sort_order) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Development', 'development', false, 0),
    ('00000000-0000-0000-0000-000000000001', 'Staging', 'staging', false, 1),
    ('00000000-0000-0000-0000-000000000001', 'Production', 'production', true, 2)
ON CONFLICT DO NOTHING;
