DROP TRIGGER IF EXISTS trg_immutable_audit_no_delete ON immutable_audit_log;
DROP TRIGGER IF EXISTS trg_immutable_audit_no_update ON immutable_audit_log;
DROP FUNCTION IF EXISTS prevent_immutable_audit_modification();
DROP INDEX IF EXISTS idx_immutable_audit_org_created;
DROP TABLE IF EXISTS immutable_audit_log;
