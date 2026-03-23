-- Rate limit counters for distributed sliding window rate limiting.
-- UNLOGGED: skips WAL for performance. Data loss on crash is acceptable
-- since counters are ephemeral and rebuilt from local state on sync.
CREATE UNLOGGED TABLE rate_limit_counters (
    key TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (key, window_start)
);

CREATE INDEX idx_rate_limit_counters_key ON rate_limit_counters (key);
