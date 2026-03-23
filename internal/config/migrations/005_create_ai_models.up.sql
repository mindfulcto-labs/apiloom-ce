CREATE TABLE ai_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NOT NULL,
    model_name TEXT NOT NULL,
    display_name TEXT,
    input_price_per_token NUMERIC(20,10),
    output_price_per_token NUMERIC(20,10),
    capabilities TEXT[] DEFAULT '{}',
    is_active BOOLEAN DEFAULT true,
    weight INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider, model_name)
);
