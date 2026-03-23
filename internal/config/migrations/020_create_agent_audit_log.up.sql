-- Create agent_audit_log table for immutable agent action audit trail.
CREATE TABLE IF NOT EXISTS agent_audit_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    action     VARCHAR(50) NOT NULL,
    details    JSONB,
    ip         TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying by agent + time range (most common query pattern).
CREATE INDEX IF NOT EXISTS idx_agent_audit_log_agent_created
    ON agent_audit_log (agent_id, created_at DESC);

-- Index on org_id for org-scoped audit queries.
CREATE INDEX IF NOT EXISTS idx_agent_audit_log_org_id
    ON agent_audit_log (org_id);

-- Prevent UPDATE and DELETE on audit log rows (append-only enforcement).
CREATE OR REPLACE FUNCTION prevent_audit_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'agent_audit_log is append-only: % not allowed', TG_OP;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_agent_audit_no_update
    BEFORE UPDATE ON agent_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_modification();

CREATE TRIGGER trg_agent_audit_no_delete
    BEFORE DELETE ON agent_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_modification();
