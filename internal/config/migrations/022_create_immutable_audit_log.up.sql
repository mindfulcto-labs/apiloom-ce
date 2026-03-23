-- Immutable audit log with cryptographic hash chaining for tamper detection.
CREATE TABLE IF NOT EXISTS immutable_audit_log (
    id            UUID PRIMARY KEY,
    org_id        UUID NOT NULL,
    actor_type    VARCHAR(10) NOT NULL CHECK (actor_type IN ('user', 'agent', 'system')),
    actor_id      TEXT NOT NULL,
    action        VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL DEFAULT '',
    resource_id   TEXT NOT NULL DEFAULT '',
    details       JSONB,
    ip            TEXT NOT NULL DEFAULT '',
    prev_hash     TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for org-scoped queries ordered by time (most common access pattern).
CREATE INDEX IF NOT EXISTS idx_immutable_audit_org_created
    ON immutable_audit_log (org_id, created_at DESC);

-- Prevent UPDATE on immutable audit log rows (append-only enforcement).
CREATE OR REPLACE FUNCTION prevent_immutable_audit_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'immutable_audit_log is append-only: % not allowed', TG_OP;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_immutable_audit_no_update
    BEFORE UPDATE ON immutable_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_immutable_audit_modification();

CREATE TRIGGER trg_immutable_audit_no_delete
    BEFORE DELETE ON immutable_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_immutable_audit_modification();
