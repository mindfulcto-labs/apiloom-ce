-- 009_create_organizations.down.sql
-- Drops the multi-tenancy tables in reverse dependency order.

DROP TRIGGER IF EXISTS trg_organizations_updated_at ON organizations;
DROP INDEX IF EXISTS idx_members_user;
DROP INDEX IF EXISTS idx_members_org;
DROP INDEX IF EXISTS idx_teams_org;
DROP TABLE IF EXISTS members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS organizations;
