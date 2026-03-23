CREATE TABLE billing_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    consumer_id UUID REFERENCES consumers(id),
    api_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL DEFAULT '',
    status_code INT NOT NULL DEFAULT 200,
    latency_ms BIGINT NOT NULL DEFAULT 0,
    tokens_used BIGINT NOT NULL DEFAULT 0,
    bytes_in BIGINT NOT NULL DEFAULT 0,
    bytes_out BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX idx_billing_outbox_unsent ON billing_outbox (created_at) WHERE sent_at IS NULL;
CREATE INDEX idx_billing_outbox_org ON billing_outbox (organization_id, created_at);
