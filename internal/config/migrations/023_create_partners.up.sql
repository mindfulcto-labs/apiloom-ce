CREATE TABLE IF NOT EXISTS partners (
    id          UUID PRIMARY KEY,
    org_id      UUID NOT NULL,
    company_name TEXT NOT NULL,
    contact_email TEXT NOT NULL,
    website     TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending',
    tier_id     TEXT NOT NULL DEFAULT 'free',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_partners_org_id ON partners (org_id);
CREATE INDEX idx_partners_status ON partners (status);

CREATE TABLE IF NOT EXISTS marketplace_listings (
    id            UUID PRIMARY KEY,
    partner_id    UUID NOT NULL REFERENCES partners(id),
    title         TEXT NOT NULL,
    description   TEXT NOT NULL,
    version       TEXT NOT NULL,
    category      TEXT NOT NULL,
    pricing_model TEXT NOT NULL DEFAULT 'free',
    price         BIGINT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'draft',
    endpoints     JSONB,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_marketplace_listings_partner_id ON marketplace_listings (partner_id);
CREATE INDEX idx_marketplace_listings_category ON marketplace_listings (category);
CREATE INDEX idx_marketplace_listings_status ON marketplace_listings (status);
-- gin_trgm_ops requires pg_trgm extension. Create conditionally.
DO $$ BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
  CREATE INDEX IF NOT EXISTS idx_marketplace_listings_title_trgm ON marketplace_listings USING gin (title gin_trgm_ops);
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_trgm not available, skipping trigram index';
END $$;
