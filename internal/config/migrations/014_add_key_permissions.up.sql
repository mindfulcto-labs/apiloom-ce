-- 014_add_key_permissions.up.sql
-- Adds granular scope-based permissions to API keys.
-- NULL permissions = unrestricted (full access). Non-null = restricted to listed scopes.

ALTER TABLE api_keys ADD COLUMN permissions JSONB;

COMMENT ON COLUMN api_keys.permissions IS 'JSON array of scope strings. NULL = unrestricted.';

CREATE INDEX idx_api_keys_permissions ON api_keys USING gin (permissions) WHERE permissions IS NOT NULL;
