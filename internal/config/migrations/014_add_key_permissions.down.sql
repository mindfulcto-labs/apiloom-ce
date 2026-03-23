-- 014_add_key_permissions.down.sql
-- Removes permissions column from api_keys.

DROP INDEX IF EXISTS idx_api_keys_permissions;

ALTER TABLE api_keys DROP COLUMN permissions;
