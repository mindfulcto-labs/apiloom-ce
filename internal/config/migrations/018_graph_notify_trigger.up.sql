-- Add NOTIFY trigger on relationships table for graph sync (if table exists).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'relationships') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_relationships_notify') THEN
      CREATE TRIGGER trg_relationships_notify
          AFTER INSERT OR UPDATE OR DELETE ON relationships
          FOR EACH ROW
          EXECUTE FUNCTION notify_config_change();
    END IF;
  END IF;
END
$$;
