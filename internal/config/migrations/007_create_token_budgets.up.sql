CREATE TABLE token_budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_id UUID REFERENCES consumers(id),
    team TEXT,
    period TEXT NOT NULL CHECK (period IN ('hour', 'day', 'month')),
    max_tokens BIGINT,
    max_cost_usd NUMERIC(10,2),
    current_tokens BIGINT DEFAULT 0,
    current_cost_usd NUMERIC(10,6) DEFAULT 0,
    period_start TIMESTAMPTZ DEFAULT NOW(),
    enforcement TEXT DEFAULT 'hard' CHECK (enforcement IN ('hard', 'soft')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CHECK (consumer_id IS NOT NULL OR team IS NOT NULL)
);

CREATE UNIQUE INDEX idx_token_budgets_consumer_period ON token_budgets (consumer_id, period) WHERE consumer_id IS NOT NULL;
CREATE UNIQUE INDEX idx_token_budgets_team_period ON token_budgets (team, period) WHERE team IS NOT NULL;
