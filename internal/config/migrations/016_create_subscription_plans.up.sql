CREATE TABLE subscription_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    tier TEXT NOT NULL CHECK (tier IN ('community','pro','business','enterprise')),
    price_monthly INT NOT NULL DEFAULT 0,
    request_limit BIGINT NOT NULL DEFAULT 0,
    rate_limit_rps INT NOT NULL DEFAULT 0,
    features JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    plan_id UUID NOT NULL REFERENCES subscription_plans(id),
    stripe_subscription_id TEXT NOT NULL DEFAULT '',
    stripe_customer_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','canceled','past_due','trialing')),
    current_period_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    current_period_end TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_subscriptions_org ON subscriptions (organization_id) WHERE status = 'active';

CREATE TABLE budget_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    threshold INT NOT NULL CHECK (threshold IN (80, 90, 100)),
    notified_at TIMESTAMPTZ,
    period_start TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, threshold, period_start)
);

-- Seed default plans
INSERT INTO subscription_plans (name, slug, tier, price_monthly, request_limit, rate_limit_rps, features) VALUES
    ('Community', 'community', 'community', 0, 10000, 10, '{"support":"community"}'),
    ('Pro', 'pro', 'pro', 9900, 500000, 100, '{"support":"email","analytics":"advanced"}'),
    ('Business', 'business', 'business', 49900, 5000000, 500, '{"support":"priority","analytics":"advanced","sla":"99.9"}'),
    ('Enterprise', 'enterprise', 'enterprise', 0, 0, 0, '{"support":"dedicated","analytics":"custom","sla":"99.99"}');
