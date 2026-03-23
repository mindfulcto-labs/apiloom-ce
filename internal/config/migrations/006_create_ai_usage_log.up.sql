CREATE TABLE ai_usage_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_id UUID REFERENCES consumers(id),
    route_id UUID,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_tokens INTEGER,
    completion_tokens INTEGER,
    total_tokens INTEGER,
    cost_usd NUMERIC(10,6),
    latency_ms INTEGER,
    is_streaming BOOLEAN DEFAULT false,
    is_cache_hit BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_ai_usage_log_consumer_created ON ai_usage_log (consumer_id, created_at DESC);
CREATE INDEX idx_ai_usage_log_provider_model_created ON ai_usage_log (provider, model, created_at DESC);
