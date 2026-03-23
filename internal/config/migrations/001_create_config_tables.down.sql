-- 001_create_config_tables.down.sql
-- Drops all config tables, triggers, and functions.

DROP TRIGGER IF EXISTS trg_backends_notify ON backends;
DROP TRIGGER IF EXISTS trg_routes_notify ON routes;
DROP FUNCTION IF EXISTS notify_config_change();

DROP TRIGGER IF EXISTS trg_backends_updated_at ON backends;
DROP TRIGGER IF EXISTS trg_routes_updated_at ON routes;
DROP FUNCTION IF EXISTS update_updated_at();

DROP TABLE IF EXISTS relationships;
DROP TABLE IF EXISTS backends;
DROP TABLE IF EXISTS routes;
