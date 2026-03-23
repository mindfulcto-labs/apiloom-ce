DROP TRIGGER IF EXISTS trg_agent_audit_no_delete ON agent_audit_log;
DROP TRIGGER IF EXISTS trg_agent_audit_no_update ON agent_audit_log;
DROP FUNCTION IF EXISTS prevent_audit_modification();
DROP TABLE IF EXISTS agent_audit_log;
