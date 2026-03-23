-- 009_create_organizations.up.sql
-- Creates the multi-tenancy foundation: organizations, teams, and members tables.

-- organizations table
CREATE TABLE IF NOT EXISTS organizations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    slug          VARCHAR(255) NOT NULL UNIQUE,
    metadata      JSONB DEFAULT '{}',
    settings      JSONB DEFAULT '{}',
    billing_email VARCHAR(255),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- teams table
CREATE TABLE IF NOT EXISTS teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, name)
);

-- members table
CREATE TABLE IF NOT EXISTS members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    team_id         UUID REFERENCES teams(id) ON DELETE SET NULL,
    user_id         TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('platform_admin', 'org_admin', 'developer', 'viewer')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, user_id)
);

CREATE INDEX idx_teams_org ON teams(organization_id);
CREATE INDEX idx_members_org ON members(organization_id);
CREATE INDEX idx_members_user ON members(user_id);

CREATE TRIGGER trg_organizations_updated_at
    BEFORE UPDATE ON organizations FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();
