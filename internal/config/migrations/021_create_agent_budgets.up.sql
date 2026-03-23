-- Agent behavioral budgets: per-agent, per-type budget tracking with time periods.
CREATE TABLE IF NOT EXISTS agent_budgets (
    agent_id    UUID        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    budget_type TEXT        NOT NULL CHECK (budget_type IN ('operations', 'tokens', 'cost')),
    limit_value DOUBLE PRECISION NOT NULL DEFAULT 0,
    used_value  DOUBLE PRECISION NOT NULL DEFAULT 0,
    period      TEXT        NOT NULL CHECK (period IN ('hourly', 'daily')) DEFAULT 'hourly',
    period_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    period_end   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '1 hour',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (agent_id, budget_type)
);

-- Index for fast lookups by agent.
CREATE INDEX IF NOT EXISTS idx_agent_budgets_agent_id ON agent_budgets(agent_id);

-- Trigger to update updated_at on modification.
CREATE OR REPLACE FUNCTION update_agent_budgets_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_agent_budgets_updated_at
    BEFORE UPDATE ON agent_budgets
    FOR EACH ROW
    EXECUTE FUNCTION update_agent_budgets_updated_at();
