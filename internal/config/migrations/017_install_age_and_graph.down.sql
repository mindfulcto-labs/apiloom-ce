-- Down migration: Remove ontology graph and AGE extension.
-- drop_graph with cascade=true removes all labels, indexes, and data.

LOAD 'age';
SET search_path TO ag_catalog, "$user", public;

SELECT drop_graph('flexgateway_ontology', true);
DROP EXTENSION IF EXISTS age;
