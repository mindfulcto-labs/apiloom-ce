-- Migration 017: Install Apache AGE extension and create ontology graph schema.
-- Skips gracefully if AGE extension is not installed (standard postgres:16-alpine).
-- AGE features require a Postgres image with AGE (e.g., apache/age:PG16_latest).

DO $$
BEGIN
  -- Check if AGE extension is available before attempting to create it.
  IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'age') THEN
    CREATE EXTENSION IF NOT EXISTS age;
    PERFORM set_config('search_path', 'ag_catalog, ' || current_setting('search_path'), false);

    -- Create the ontology graph if it doesn't exist.
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'flexgateway_ontology') THEN
      PERFORM create_graph('flexgateway_ontology');
    END IF;

    -- Vertex labels (13 node types).
    PERFORM create_vlabel('flexgateway_ontology', 'API');
    PERFORM create_vlabel('flexgateway_ontology', 'Service');
    PERFORM create_vlabel('flexgateway_ontology', 'Route');
    PERFORM create_vlabel('flexgateway_ontology', 'Consumer');
    PERFORM create_vlabel('flexgateway_ontology', 'Team');
    PERFORM create_vlabel('flexgateway_ontology', 'Org');
    PERFORM create_vlabel('flexgateway_ontology', 'Model');
    PERFORM create_vlabel('flexgateway_ontology', 'Agent');
    PERFORM create_vlabel('flexgateway_ontology', 'Policy');
    PERFORM create_vlabel('flexgateway_ontology', 'Plugin');
    PERFORM create_vlabel('flexgateway_ontology', 'Gateway');
    PERFORM create_vlabel('flexgateway_ontology', 'Environment');
    PERFORM create_vlabel('flexgateway_ontology', 'Version');

    -- Edge labels (9 edge types).
    PERFORM create_elabel('flexgateway_ontology', 'owns');
    PERFORM create_elabel('flexgateway_ontology', 'subscribes_to');
    PERFORM create_elabel('flexgateway_ontology', 'depends_on');
    PERFORM create_elabel('flexgateway_ontology', 'routes_to');
    PERFORM create_elabel('flexgateway_ontology', 'applies_to');
    PERFORM create_elabel('flexgateway_ontology', 'consumes');
    PERFORM create_elabel('flexgateway_ontology', 'authorizes');
    PERFORM create_elabel('flexgateway_ontology', 'budgets');
    PERFORM create_elabel('flexgateway_ontology', 'data_flows_to');

    RAISE NOTICE 'Apache AGE ontology graph created successfully';
  ELSE
    RAISE NOTICE 'Apache AGE extension not available — skipping ontology graph creation. Use apache/age:PG16_latest image for AGE support.';
  END IF;
END
$$;
