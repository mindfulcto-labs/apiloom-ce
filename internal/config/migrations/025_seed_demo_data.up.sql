-- 025_seed_demo_data.up.sql
-- Seed realistic demo data for Acme Corp, a fictional SaaS company using Apiloom.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- Organization: update default org to Acme Corp branding
-- =============================================================================
UPDATE organizations
SET name = 'Acme Corp',
    slug = 'acme-corp',
    billing_email = 'billing@acme.corp',
    metadata = '{"industry": "saas", "plan": "enterprise", "seats": 45}'::jsonb,
    settings = '{"default_rate_limit": 1000, "max_routes": 100, "sso_enabled": true, "audit_retention_days": 90}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000001';

-- =============================================================================
-- Backends (8 services)
-- =============================================================================
INSERT INTO backends (id, name, config, metadata, organization_id) VALUES
('b0000000-0000-0000-0000-000000000001', 'users-service', '{
  "url": "http://users-svc.internal:8080",
  "timeout": "5s",
  "retry": {"attempts": 3, "backoff": "100ms"},
  "health_check": {"path": "/healthz", "interval": "10s", "threshold": 3}
}'::jsonb, '{"team": "platform", "environment": "production", "language": "go", "version": "2.14.0"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000002', 'orders-service', '{
  "url": "http://orders-svc.internal:8080",
  "timeout": "10s",
  "retry": {"attempts": 2, "backoff": "200ms"},
  "health_check": {"path": "/healthz", "interval": "15s", "threshold": 3}
}'::jsonb, '{"team": "commerce", "environment": "production", "language": "java", "version": "3.7.2"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000003', 'payments-service', '{
  "url": "http://payments-svc.internal:8443",
  "timeout": "15s",
  "retry": {"attempts": 1, "backoff": "500ms"},
  "health_check": {"path": "/health", "interval": "10s", "threshold": 2},
  "tls": {"enabled": true, "verify": true}
}'::jsonb, '{"team": "finops", "environment": "production", "language": "go", "version": "1.9.1", "pci_compliant": true}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000004', 'auth-service', '{
  "url": "http://auth-svc.internal:8080",
  "timeout": "3s",
  "retry": {"attempts": 2, "backoff": "50ms"},
  "health_check": {"path": "/healthz", "interval": "5s", "threshold": 2}
}'::jsonb, '{"team": "security", "environment": "production", "language": "go", "version": "4.2.0"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000005', 'search-service', '{
  "url": "http://search-svc.internal:9200",
  "timeout": "8s",
  "retry": {"attempts": 2, "backoff": "150ms"},
  "health_check": {"path": "/_cluster/health", "interval": "30s", "threshold": 3}
}'::jsonb, '{"team": "platform", "environment": "production", "language": "python", "version": "1.5.3"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000006', 'ai-service', '{
  "url": "http://ai-gateway.internal:8080",
  "timeout": "120s",
  "retry": {"attempts": 1, "backoff": "1s"},
  "health_check": {"path": "/healthz", "interval": "30s", "threshold": 5},
  "streaming": true
}'::jsonb, '{"team": "ml-platform", "environment": "production", "language": "python", "version": "0.8.4"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000007', 'notifications-service', '{
  "url": "http://notifications-svc.internal:8080",
  "timeout": "5s",
  "retry": {"attempts": 3, "backoff": "200ms"},
  "health_check": {"path": "/healthz", "interval": "15s", "threshold": 3}
}'::jsonb, '{"team": "platform", "environment": "production", "language": "typescript", "version": "2.1.0"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

('b0000000-0000-0000-0000-000000000008', 'analytics-service', '{
  "url": "http://analytics-svc.internal:8080",
  "timeout": "20s",
  "retry": {"attempts": 2, "backoff": "300ms"},
  "health_check": {"path": "/healthz", "interval": "30s", "threshold": 3}
}'::jsonb, '{"team": "data", "environment": "production", "language": "go", "version": "1.3.7"}'::jsonb, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Routes (18 routes)
-- =============================================================================
INSERT INTO routes (id, name, version, config, metadata, checksum, is_active, organization_id) VALUES
('a0000000-0000-0000-0000-000000000001', 'users-list', 3, '{
  "endpoint": "/api/v1/users",
  "method": "GET",
  "backend_url": "http://users-svc.internal:8080/users",
  "timeout": "5s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["users", "core"], "environment": "production", "team": "platform", "description": "List and search users"}'::jsonb,
'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000002', 'users-crud', 5, '{
  "endpoint": "/api/v1/users/{id}",
  "method": "GET,PUT,PATCH,DELETE",
  "backend_url": "http://users-svc.internal:8080/users/{id}",
  "timeout": "5s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["users", "core"], "environment": "production", "team": "platform", "description": "User CRUD operations"}'::jsonb,
'a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000003', 'orders-list', 2, '{
  "endpoint": "/api/v1/orders",
  "method": "GET,POST",
  "backend_url": "http://orders-svc.internal:8080/orders",
  "timeout": "10s",
  "blocks": ["auth-jwt", "rate-limit", "cors", "transform"]
}'::jsonb, '{"tags": ["orders", "commerce"], "environment": "production", "team": "commerce", "description": "Create and list orders"}'::jsonb,
'b2c3d4e5f6a789012345678901234567890abcdef1234567890abcdef1234567', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000004', 'orders-detail', 2, '{
  "endpoint": "/api/v1/orders/{id}",
  "method": "GET,PUT,DELETE",
  "backend_url": "http://orders-svc.internal:8080/orders/{id}",
  "timeout": "10s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["orders", "commerce"], "environment": "production", "team": "commerce", "description": "Order detail operations"}'::jsonb,
'c3d4e5f6a7b89012345678901234567890abcdef1234567890abcdef12345678', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000005', 'products-catalog', 4, '{
  "endpoint": "/api/v1/products",
  "method": "GET",
  "backend_url": "http://orders-svc.internal:8080/products",
  "timeout": "8s",
  "blocks": ["rate-limit", "cors"],
  "cache_ttl": "300s"
}'::jsonb, '{"tags": ["products", "commerce", "public"], "environment": "production", "team": "commerce", "description": "Public product catalog"}'::jsonb,
'd4e5f6a7b8c9012345678901234567890abcdef1234567890abcdef123456789', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000006', 'products-detail', 4, '{
  "endpoint": "/api/v1/products/{id}",
  "method": "GET,PUT,POST,DELETE",
  "backend_url": "http://orders-svc.internal:8080/products/{id}",
  "timeout": "8s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["products", "commerce"], "environment": "production", "team": "commerce", "description": "Product management"}'::jsonb,
'e5f6a7b8c9d0123456789012345678901abcdef1234567890abcdef123456789', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000007', 'payments-process', 2, '{
  "endpoint": "/api/v1/payments",
  "method": "POST",
  "backend_url": "http://payments-svc.internal:8443/payments",
  "timeout": "15s",
  "blocks": ["auth-jwt", "rate-limit", "cors", "circuit-breaker"],
  "tls": true
}'::jsonb, '{"tags": ["payments", "finops", "pci"], "environment": "production", "team": "finops", "description": "Process payment transactions"}'::jsonb,
'f6a7b8c9d0e12345678901234567890123abcdef1234567890abcdef12345678', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000008', 'payments-history', 1, '{
  "endpoint": "/api/v1/payments/{id}",
  "method": "GET",
  "backend_url": "http://payments-svc.internal:8443/payments/{id}",
  "timeout": "10s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["payments", "finops"], "environment": "production", "team": "finops", "description": "Payment detail and history"}'::jsonb,
'a7b8c9d0e1f2345678901234567890abcd1234567890abcdef1234567890abcd', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000009', 'auth-login', 7, '{
  "endpoint": "/api/v1/auth/login",
  "method": "POST",
  "backend_url": "http://auth-svc.internal:8080/login",
  "timeout": "3s",
  "blocks": ["rate-limit", "cors"],
  "rate_limit_override": {"requests_per_minute": 20}
}'::jsonb, '{"tags": ["auth", "security", "public"], "environment": "production", "team": "security", "description": "User authentication"}'::jsonb,
'b8c9d0e1f2a345678901234567890abcde234567890abcdef1234567890abcde', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000010', 'auth-register', 3, '{
  "endpoint": "/api/v1/auth/register",
  "method": "POST",
  "backend_url": "http://auth-svc.internal:8080/register",
  "timeout": "5s",
  "blocks": ["rate-limit", "cors"],
  "rate_limit_override": {"requests_per_minute": 10}
}'::jsonb, '{"tags": ["auth", "security", "public"], "environment": "production", "team": "security", "description": "User registration"}'::jsonb,
'c9d0e1f2a3b45678901234567890abcdef34567890abcdef1234567890abcdef', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000011', 'search-query', 2, '{
  "endpoint": "/api/v1/search",
  "method": "GET,POST",
  "backend_url": "http://search-svc.internal:9200/search",
  "timeout": "8s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["search", "platform"], "environment": "production", "team": "platform", "description": "Full-text search across entities"}'::jsonb,
'd0e1f2a3b4c5678901234567890abcdef4567890abcdef1234567890abcdef12', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000012', 'notifications-send', 1, '{
  "endpoint": "/api/v1/notifications",
  "method": "GET,POST",
  "backend_url": "http://notifications-svc.internal:8080/notifications",
  "timeout": "5s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["notifications", "platform"], "environment": "production", "team": "platform", "description": "Send and list notifications"}'::jsonb,
'e1f2a3b4c5d6789012345678901abcdef567890abcdef1234567890abcdef123', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000013', 'analytics-events', 2, '{
  "endpoint": "/api/v1/analytics",
  "method": "GET,POST",
  "backend_url": "http://analytics-svc.internal:8080/events",
  "timeout": "20s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["analytics", "data"], "environment": "production", "team": "data", "description": "Analytics event ingestion and queries"}'::jsonb,
'f2a3b4c5d6e789012345678901abcdef6789012bcdef1234567890abcdef1234', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000014', 'webhooks-manage', 1, '{
  "endpoint": "/api/v1/webhooks",
  "method": "GET,POST,PUT,DELETE",
  "backend_url": "http://notifications-svc.internal:8080/webhooks",
  "timeout": "5s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["webhooks", "platform"], "environment": "production", "team": "platform", "description": "Webhook endpoint management"}'::jsonb,
'a3b4c5d6e7f89012345678901abcdef78901234cdef1234567890abcdef12345', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000015', 'ai-completions', 3, '{
  "endpoint": "/api/v1/ai/completions",
  "method": "POST",
  "backend_url": "http://ai-gateway.internal:8080/completions",
  "timeout": "120s",
  "blocks": ["auth-jwt", "rate-limit", "cors", "ai-proxy"],
  "streaming": true
}'::jsonb, '{"tags": ["ai", "ml-platform", "llm"], "environment": "production", "team": "ml-platform", "description": "LLM chat completions with streaming"}'::jsonb,
'b4c5d6e7f8a901234567890123abcdef890123456def1234567890abcdef1234', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000016', 'ai-embeddings', 2, '{
  "endpoint": "/api/v1/ai/embeddings",
  "method": "POST",
  "backend_url": "http://ai-gateway.internal:8080/embeddings",
  "timeout": "30s",
  "blocks": ["auth-jwt", "rate-limit", "cors", "ai-proxy"]
}'::jsonb, '{"tags": ["ai", "ml-platform", "embeddings"], "environment": "production", "team": "ml-platform", "description": "Text embedding generation"}'::jsonb,
'c5d6e7f8a9b01234567890abcdef012345678901ef1234567890abcdef123456', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000017', 'files-upload', 1, '{
  "endpoint": "/api/v1/files",
  "method": "GET,POST,DELETE",
  "backend_url": "http://users-svc.internal:8080/files",
  "timeout": "30s",
  "blocks": ["auth-jwt", "rate-limit", "cors"],
  "max_body_size": "50MB"
}'::jsonb, '{"tags": ["files", "platform"], "environment": "production", "team": "platform", "description": "File upload and management"}'::jsonb,
'd6e7f8a9b0c12345678901abcdef01234567890123567890abcdef1234567890', true, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000018', 'billing-api', 2, '{
  "endpoint": "/api/v1/billing",
  "method": "GET,POST",
  "backend_url": "http://payments-svc.internal:8443/billing",
  "timeout": "10s",
  "blocks": ["auth-jwt", "rate-limit", "cors"]
}'::jsonb, '{"tags": ["billing", "finops"], "environment": "production", "team": "finops", "description": "Billing and subscription management"}'::jsonb,
'e7f8a9b0c1d2345678901abcdef0123456789012345890abcdef12345678901a', false, '00000000-0000-0000-0000-000000000001'),

('a0000000-0000-0000-0000-000000000019', 'health-check', 1, '{
  "endpoint": "/api/v1/health",
  "method": "GET",
  "backend_url": "http://localhost:8080/healthz",
  "timeout": "2s",
  "blocks": ["cors"]
}'::jsonb, '{"tags": ["health", "ops", "public"], "environment": "production", "team": "platform", "description": "Gateway health check endpoint"}'::jsonb,
'f8a9b0c1d2e3456789012abcdef01234567890123456abcdef123456789012bc', true, '00000000-0000-0000-0000-000000000001'),

-- GraphQL endpoint (multi-protocol proxy)
('a0000000-0000-0000-0000-000000000020', 'graphql-gateway', 5, '{
  "endpoint": "/graphql",
  "method": "POST",
  "protocol": "graphql",
  "backend_url": "http://graphql-federation.internal:4000/graphql",
  "timeout": "15s",
  "blocks": ["auth-jwt", "rate-limit", "cors"],
  "graphql": {"introspection": false, "max_depth": 10, "max_complexity": 500, "persisted_queries": true}
}'::jsonb, '{"tags": ["graphql", "federation", "core"], "environment": "production", "team": "platform", "description": "GraphQL federated gateway — schema stitching across 4 subgraphs", "api_type": "graphql"}'::jsonb,
'aa11bb22cc33dd44ee55ff6677889900aabbccdd11223344556677889900aabb', true, '00000000-0000-0000-0000-000000000001'),

-- WebSocket streaming endpoint
('a0000000-0000-0000-0000-000000000021', 'realtime-ws', 2, '{
  "endpoint": "/ws/v1/events",
  "method": "GET",
  "protocol": "websocket",
  "backend_url": "ws://realtime-svc.internal:8080/events",
  "timeout": "0",
  "blocks": ["auth-jwt", "rate-limit"],
  "websocket": {"ping_interval": "30s", "max_message_size": "1MB", "compression": true}
}'::jsonb, '{"tags": ["websocket", "realtime", "streaming"], "environment": "production", "team": "platform", "description": "Real-time event streaming via WebSocket", "api_type": "websocket"}'::jsonb,
'bb22cc33dd44ee55ff6677889900aabb11223344556677889900aabbccddeeff', true, '00000000-0000-0000-0000-000000000001'),

-- gRPC proxy endpoint
('a0000000-0000-0000-0000-000000000022', 'grpc-inference', 1, '{
  "endpoint": "/grpc/ml.InferenceService",
  "method": "POST",
  "protocol": "grpc",
  "backend_url": "grpc://ml-inference.internal:50051",
  "timeout": "60s",
  "blocks": ["auth-jwt", "rate-limit"],
  "grpc": {"reflection": true, "max_message_size": "10MB", "keepalive": "60s"}
}'::jsonb, '{"tags": ["grpc", "ml", "inference"], "environment": "production", "team": "ml-platform", "description": "gRPC ML model inference service", "api_type": "grpc"}'::jsonb,
'cc33dd44ee55ff6677889900aabb112233445566778899aabbccddeeff001122', true, '00000000-0000-0000-0000-000000000001'),

-- Server-Sent Events endpoint
('a0000000-0000-0000-0000-000000000023', 'sse-dashboard-feed', 1, '{
  "endpoint": "/api/v1/events/stream",
  "method": "GET",
  "protocol": "sse",
  "backend_url": "http://analytics-svc.internal:8080/events/stream",
  "timeout": "0",
  "blocks": ["auth-jwt", "cors"],
  "sse": {"retry_interval": "3000", "heartbeat": "15s"}
}'::jsonb, '{"tags": ["sse", "streaming", "analytics"], "environment": "production", "team": "data", "description": "Server-Sent Events for live dashboard metrics", "api_type": "sse"}'::jsonb,
'dd44ee55ff6677889900aabb11223344556677889900aabbccddeeff00112233', true, '00000000-0000-0000-0000-000000000001'),

-- REST + OpenAPI documented endpoint
('a0000000-0000-0000-0000-000000000024', 'partner-api-v2', 3, '{
  "endpoint": "/partner/v2/*",
  "method": "GET,POST,PUT,DELETE",
  "backend_url": "http://orders-svc.internal:8080/partner/v2",
  "timeout": "10s",
  "blocks": ["auth-jwt", "rate-limit", "cors", "transform"],
  "openapi_spec": "/specs/partner-v2.yaml",
  "versioning": {"strategy": "url-path", "deprecated_versions": ["v1"]}
}'::jsonb, '{"tags": ["partner", "external", "versioned"], "environment": "production", "team": "commerce", "description": "Partner integration REST API v2 — OpenAPI documented", "api_type": "rest"}'::jsonb,
'ee55ff6677889900aabb1122334455667788990011223344556677889900aabb', true, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Consumers (10 consumers)
-- =============================================================================
INSERT INTO consumers (id, name, metadata, is_active, organization_id) VALUES
('c0000000-0000-0000-0000-000000000001', 'mobile-app', '{"team": "mobile", "environment": "production", "contact_email": "mobile-team@acme.corp", "plan": "enterprise", "platform": "ios,android"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000002', 'web-dashboard', '{"team": "frontend", "environment": "production", "contact_email": "frontend@acme.corp", "plan": "enterprise", "framework": "next.js"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000003', 'partner-integration', '{"team": "partnerships", "environment": "production", "contact_email": "partners@acme.corp", "plan": "partner", "partner_name": "TechFlow Inc"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000004', 'data-pipeline', '{"team": "data", "environment": "production", "contact_email": "data-eng@acme.corp", "plan": "internal", "scheduler": "airflow"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000005', 'monitoring-agent', '{"team": "sre", "environment": "production", "contact_email": "sre@acme.corp", "plan": "internal", "tool": "datadog"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000006', 'ci-cd-pipeline', '{"team": "devops", "environment": "production", "contact_email": "devops@acme.corp", "plan": "internal", "tool": "github-actions"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000007', 'analytics-service', '{"team": "data", "environment": "production", "contact_email": "analytics@acme.corp", "plan": "internal"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000008', 'billing-service', '{"team": "finops", "environment": "production", "contact_email": "billing@acme.corp", "plan": "internal"}'::jsonb, true, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000009', 'third-party-widget', '{"team": "partnerships", "environment": "production", "contact_email": "widgets@acme.corp", "plan": "free", "partner_name": "WidgetCo"}'::jsonb, false, '00000000-0000-0000-0000-000000000001'),
('c0000000-0000-0000-0000-000000000010', 'internal-tools', '{"team": "platform", "environment": "production", "contact_email": "platform@acme.corp", "plan": "internal", "tools": ["admin-panel", "debug-console"]}'::jsonb, true, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- API Keys (14 keys)
-- =============================================================================
INSERT INTO api_keys (id, consumer_id, key_hash, key_prefix, is_active, expires_at, last_used_at, organization_id) VALUES
-- mobile-app keys
('a1000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001',
 '$2a$12$LJ3m4ys3Gz5G6wJkLXxHZ.QG7jR8e9R4yKzN1vM2bC3dE4fG5hI6j', 'flex_ak_Xm7k', true,
 NOW() + INTERVAL '90 days', NOW() - INTERVAL '12 minutes', '00000000-0000-0000-0000-000000000001'),

('a1000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000001',
 '$2a$12$aB3cD4eF5gH6iJ7kL8mN9.oP0qR1sT2uV3wX4yZ5aB6cD7eF8gH9i', 'flex_ak_9pQr', true,
 NOW() + INTERVAL '180 days', NOW() - INTERVAL '3 hours', '00000000-0000-0000-0000-000000000001'),

-- web-dashboard keys
('a1000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000002',
 '$2a$12$jK4lM5nO6pQ7rS8tU9vW0.xY1zA2bC3dE4fG5hI6jK7lM8nO9pQ0r', 'flex_ak_Wn3t', true,
 NOW() + INTERVAL '60 days', NOW() - INTERVAL '2 minutes', '00000000-0000-0000-0000-000000000001'),

-- partner-integration keys
('a1000000-0000-0000-0000-000000000004', 'c0000000-0000-0000-0000-000000000003',
 '$2a$12$sT5uV6wX7yZ8aB9cD0eF1.gH2iJ3kL4mN5oP6qR7sT8uV9wX0yZ1a', 'flex_ak_Hp8v', true,
 NOW() + INTERVAL '365 days', NOW() - INTERVAL '45 minutes', '00000000-0000-0000-0000-000000000001'),

('a1000000-0000-0000-0000-000000000005', 'c0000000-0000-0000-0000-000000000003',
 '$2a$12$bC2dE3fG4hI5jK6lM7nO8.pQ9rS0tU1vW2xY3zA4bC5dE6fG7hI8j', 'flex_ak_Ld2m', false,
 NOW() - INTERVAL '30 days', NOW() - INTERVAL '31 days', '00000000-0000-0000-0000-000000000001'),

-- data-pipeline keys
('a1000000-0000-0000-0000-000000000006', 'c0000000-0000-0000-0000-000000000004',
 '$2a$12$kL3mN4oP5qR6sT7uV8wX9.yZ0aB1cD2eF3gH4iJ5kL6mN7oP8qR9s', 'flex_ak_Jf5x', true,
 NOW() + INTERVAL '30 days', NOW() - INTERVAL '6 hours', '00000000-0000-0000-0000-000000000001'),

-- monitoring-agent keys
('a1000000-0000-0000-0000-000000000007', 'c0000000-0000-0000-0000-000000000005',
 '$2a$12$tU4vW5xY6zA7bC8dE9fG0.hI1jK2lM3nO4pQ5rS6tU7vW8xY9zA0b', 'flex_ak_Rt4n', true,
 NOW() + INTERVAL '120 days', NOW() - INTERVAL '30 seconds', '00000000-0000-0000-0000-000000000001'),

-- ci-cd-pipeline keys
('a1000000-0000-0000-0000-000000000008', 'c0000000-0000-0000-0000-000000000006',
 '$2a$12$cD5eF6gH7iJ8kL9mN0oP1.qR2sT3uV4wX5yZ6aB7cD8eF9gH0iJ1k', 'flex_ak_Bz7w', true,
 NOW() + INTERVAL '45 days', NOW() - INTERVAL '2 hours', '00000000-0000-0000-0000-000000000001'),

-- analytics-service keys
('a1000000-0000-0000-0000-000000000009', 'c0000000-0000-0000-0000-000000000007',
 '$2a$12$lM6nO7pQ8rS9tU0vW1xY2.zA3bC4dE5fG6hI7jK8lM9nO0pQ1rS2t', 'flex_ak_Qs9c', true,
 NOW() + INTERVAL '60 days', NOW() - INTERVAL '15 minutes', '00000000-0000-0000-0000-000000000001'),

-- billing-service keys
('a1000000-0000-0000-0000-000000000010', 'c0000000-0000-0000-0000-000000000008',
 '$2a$12$uV7wX8yZ9aB0cD1eF2gH3.iJ4kL5mN6oP7qR8sT9uV0wX1yZ2aB3c', 'flex_ak_Fg6y', true,
 NOW() + INTERVAL '90 days', NOW() - INTERVAL '1 hour', '00000000-0000-0000-0000-000000000001'),

-- third-party-widget (expired key)
('a1000000-0000-0000-0000-000000000011', 'c0000000-0000-0000-0000-000000000009',
 '$2a$12$dE8fG9hI0jK1lM2nO3pQ4.rS5tU6vW7xY8zA9bC0dE1fG2hI3jK4l', 'flex_ak_Nk1p', false,
 NOW() - INTERVAL '15 days', NOW() - INTERVAL '16 days', '00000000-0000-0000-0000-000000000001'),

-- internal-tools keys
('a1000000-0000-0000-0000-000000000012', 'c0000000-0000-0000-0000-000000000010',
 '$2a$12$mN9oP0qR1sT2uV3wX4yZ5.aB6cD7eF8gH9iJ0kL1mN2oP3qR4sT5u', 'flex_ak_Yw3d', true,
 NOW() + INTERVAL '365 days', NOW() - INTERVAL '5 minutes', '00000000-0000-0000-0000-000000000001'),

-- mobile-app: legacy key (expired)
('a1000000-0000-0000-0000-000000000013', 'c0000000-0000-0000-0000-000000000001',
 '$2a$12$vW0xY1zA2bC3dE4fG5hI6.jK7lM8nO9pQ0rS1tU2vW3xY4zA5bC6d', 'flex_ak_Oo8e', false,
 NOW() - INTERVAL '60 days', NOW() - INTERVAL '61 days', '00000000-0000-0000-0000-000000000001'),

-- web-dashboard: staging key
('a1000000-0000-0000-0000-000000000014', 'c0000000-0000-0000-0000-000000000002',
 '$2a$12$eF1gH2iJ3kL4mN5oP6qR7.sT8uV9wX0yZ1aB2cD3eF4gH5iJ6kL7m', 'flex_ak_Tc2f', true,
 NOW() + INTERVAL '30 days', NOW() - INTERVAL '4 hours', '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- AI Models (7 models)
-- =============================================================================
INSERT INTO ai_models (id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, organization_id) VALUES
('d0000000-0000-0000-0000-000000000001', 'openai', 'gpt-4o', 'GPT-4o', 0.0000025000, 0.0000100000, '{text,vision,function_calling}', true, 3, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000002', 'openai', 'gpt-4o-mini', 'GPT-4o Mini', 0.0000001500, 0.0000006000, '{text,vision,function_calling}', true, 5, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000003', 'anthropic', 'claude-sonnet-4-20250514', 'Claude Sonnet 4', 0.0000030000, 0.0000150000, '{text,vision,function_calling}', true, 3, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000004', 'anthropic', 'claude-haiku-4-5-20251001', 'Claude Haiku 4.5', 0.0000008000, 0.0000040000, '{text,vision}', true, 4, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000005', 'google', 'gemini-2.0-flash', 'Gemini 2.0 Flash', 0.0000001000, 0.0000004000, '{text,vision}', true, 5, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000006', 'mistral', 'mistral-large-latest', 'Mistral Large', 0.0000020000, 0.0000060000, '{text,function_calling}', true, 2, '00000000-0000-0000-0000-000000000001'),
('d0000000-0000-0000-0000-000000000007', 'cohere', 'command-r-plus', 'Command R+', 0.0000025000, 0.0000100000, '{text,embedding}', false, 1, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Token Budgets (6 budgets)
-- =============================================================================
INSERT INTO token_budgets (id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, organization_id) VALUES
-- mobile-app: monthly budget, 72% used
('e0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', NULL, 'month', 50000000, 150.00, 36000000, 108.00, date_trunc('month', NOW()), 'hard', '00000000-0000-0000-0000-000000000001'),

-- web-dashboard: daily budget, 45% used
('e0000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000002', NULL, 'day', 5000000, 15.00, 2250000, 6.75, date_trunc('day', NOW()), 'hard', '00000000-0000-0000-0000-000000000001'),

-- partner-integration: monthly budget, 83% used (near limit)
('e0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000003', NULL, 'month', 20000000, 60.00, 16600000, 49.80, date_trunc('month', NOW()), 'hard', '00000000-0000-0000-0000-000000000001'),

-- ml-platform team: hourly budget, 30% used
('e0000000-0000-0000-0000-000000000004', NULL, 'ml-platform', 'hour', 2000000, 10.00, 600000, 3.00, date_trunc('hour', NOW()), 'soft', '00000000-0000-0000-0000-000000000001'),

-- data team: daily budget, 61% used
('e0000000-0000-0000-0000-000000000005', NULL, 'data', 'day', 10000000, 30.00, 6100000, 18.30, date_trunc('day', NOW()), 'soft', '00000000-0000-0000-0000-000000000001'),

-- data-pipeline: monthly budget, 15% used (just started)
('e0000000-0000-0000-0000-000000000006', 'c0000000-0000-0000-0000-000000000004', NULL, 'month', 100000000, 300.00, 15000000, 45.00, date_trunc('month', NOW()), 'hard', '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Plugins (6 plugins)
-- =============================================================================
INSERT INTO plugins (id, name, plugin_type, config, target, ordering, is_enabled, organization_id) VALUES
('f0000000-0000-0000-0000-000000000001', 'auth-jwt', 'native', '{
  "issuer": "https://auth.acme.corp",
  "audience": ["api.acme.corp"],
  "jwks_url": "https://auth.acme.corp/.well-known/jwks.json",
  "jwks_cache_ttl": "1h",
  "algorithms": ["RS256", "ES256"],
  "clock_skew": "30s",
  "required_claims": ["sub", "iss", "exp"]
}'::jsonb, 'global', 10, true, '00000000-0000-0000-0000-000000000001'),

('f0000000-0000-0000-0000-000000000002', 'rate-limit', 'native', '{
  "default_requests_per_minute": 1000,
  "burst_size": 50,
  "key_strategy": "consumer_id",
  "fallback_strategy": "ip",
  "sliding_window": true,
  "storage": "postgres",
  "fail_closed": true
}'::jsonb, 'global', 20, true, '00000000-0000-0000-0000-000000000001'),

('f0000000-0000-0000-0000-000000000003', 'ai-proxy', 'native', '{
  "default_model": "gpt-4o-mini",
  "fallback_models": ["gemini-2.0-flash", "claude-haiku-4-5-20251001"],
  "token_counting": true,
  "budget_enforcement": true,
  "streaming": true,
  "max_tokens_per_request": 4096,
  "timeout": "120s",
  "retry_on_rate_limit": true
}'::jsonb, 'route', 30, true, '00000000-0000-0000-0000-000000000001'),

('f0000000-0000-0000-0000-000000000004', 'cors', 'native', '{
  "allowed_origins": ["https://app.acme.corp", "https://admin.acme.corp", "https://docs.acme.corp"],
  "allowed_methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
  "allowed_headers": ["Authorization", "Content-Type", "X-Request-ID", "X-API-Key"],
  "exposed_headers": ["X-Request-ID", "X-RateLimit-Remaining", "X-RateLimit-Reset"],
  "max_age": 86400,
  "allow_credentials": true
}'::jsonb, 'global', 5, true, '00000000-0000-0000-0000-000000000001'),

('f0000000-0000-0000-0000-000000000005', 'transform', 'native', '{
  "request_headers": {"add": {"X-Gateway-Version": "1.0", "X-Forwarded-Proto": "https"}, "remove": ["X-Debug"]},
  "response_headers": {"add": {"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Strict-Transport-Security": "max-age=31536000"}, "remove": ["Server", "X-Powered-By"]},
  "strip_path_prefix": "/api/v1"
}'::jsonb, 'global', 15, true, '00000000-0000-0000-0000-000000000001'),

('f0000000-0000-0000-0000-000000000006', 'circuit-breaker', 'native', '{
  "threshold": 5,
  "timeout": "30s",
  "interval": "60s",
  "max_requests_half_open": 3,
  "trip_on": ["5xx", "timeout"],
  "notify_on_state_change": true
}'::jsonb, 'route', 25, false, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Agents (5 agents)
-- =============================================================================
INSERT INTO agents (id, org_id, name, description, owner_id, capabilities, trust_level, status, api_key_hash, metadata) VALUES
('90000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'deployment-bot',
 'Automated deployment agent for CI/CD pipelines. Handles route updates and config rollouts.',
 'c0000000-0000-0000-0000-000000000006',
 '["route.read", "route.update", "backend.read", "config.reload", "health.check"]'::jsonb,
 'elevated', 'active',
 '$2a$12$AgentHash1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
 '{"team": "devops", "pipeline": "github-actions", "last_deployment": "2026-03-22T14:30:00Z"}'::jsonb),

('90000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'monitoring-agent',
 'Infrastructure monitoring agent. Reads health metrics and route status for alerting.',
 'c0000000-0000-0000-0000-000000000005',
 '["route.read", "backend.read", "health.check", "metrics.read", "consumer.read"]'::jsonb,
 'basic', 'active',
 '$2a$12$AgentHash2xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
 '{"team": "sre", "tool": "datadog", "check_interval": "30s"}'::jsonb),

('90000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'cost-optimizer',
 'AI cost optimization agent. Analyzes token usage patterns and suggests budget adjustments.',
 'c0000000-0000-0000-0000-000000000007',
 '["token_budget.read", "ai_model.read", "ai_usage.read", "token_budget.update"]'::jsonb,
 'elevated', 'active',
 '$2a$12$AgentHash3xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
 '{"team": "ml-platform", "optimization_target": "cost", "savings_last_30d": "$1,247.50"}'::jsonb),

('90000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000001', 'security-scanner',
 'Periodic security scanning agent. Audits route configs, checks for misconfigurations.',
 'c0000000-0000-0000-0000-000000000010',
 '["route.read", "plugin.read", "consumer.read", "api_key.read", "audit.read"]'::jsonb,
 'basic', 'active',
 '$2a$12$AgentHash4xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
 '{"team": "security", "scan_schedule": "every 6h", "last_scan": "2026-03-22T12:00:00Z", "issues_found": 0}'::jsonb),

('90000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000001', 'data-sync-agent',
 'Synchronizes API configurations across environments. Handles staging-to-production promotions.',
 'c0000000-0000-0000-0000-000000000006',
 '["route.read", "route.create", "backend.read", "backend.create", "config.export", "config.import"]'::jsonb,
 'elevated', 'suspended',
 '$2a$12$AgentHash5xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
 '{"team": "devops", "source_env": "staging", "target_env": "production", "suspended_reason": "pending approval for production access"}'::jsonb)

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Webhook Endpoints (4 webhooks)
-- =============================================================================
INSERT INTO webhook_endpoints (id, organization_id, url, secret, events, description, active) VALUES
('70000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001',
 'https://hooks.slack.com/services/T0ABC/B0DEF/xyzSecretWebhookToken123',
 'whsec_acme_slack_a1b2c3d4e5f6g7h8i9j0',
 '{route.created,route.updated,route.deleted,backend.created,backend.deleted,consumer.created}',
 'Slack #api-platform channel notifications for config changes',
 true),

('70000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001',
 'https://events.pagerduty.com/integration/acme-apiloom/enqueue',
 'whsec_acme_pagerduty_k1l2m3n4o5p6q7r8s9t0',
 '{route.deleted,backend.deleted,plugin.disabled,circuit_breaker.opened,health_check.failed}',
 'PagerDuty alerts for critical infrastructure events',
 true),

('70000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001',
 'https://http-intake.logs.datadoghq.com/api/v2/logs',
 'whsec_acme_datadog_u1v2w3x4y5z6a7b8c9d0',
 '{ai.request.completed,ai.budget.exceeded,rate_limit.exceeded}',
 'DataDog metrics for AI usage and rate limiting events',
 true),

('70000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000001',
 'https://analytics.acme.corp/webhooks/apiloom',
 'whsec_acme_internal_e1f2g3h4i5j6k7l8m9n0',
 '{route.created,route.updated,consumer.created,api_key.rotated,ai.request.completed}',
 'Internal analytics pipeline for API platform metrics',
 false)

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Immutable Audit Log (40 entries spanning last 7 days)
-- =============================================================================
INSERT INTO immutable_audit_log (id, org_id, actor_type, actor_id, action, resource_type, resource_id, details, ip, prev_hash, created_at) VALUES

-- Day 7 ago: initial setup
('80000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'backend', 'b0000000-0000-0000-0000-000000000001',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "backend_name": "users-service", "backend_url": "http://users-svc.internal:8080"}'::jsonb,
 '10.0.1.50', '0000000000000000000000000000000000000000000000000000000000000000',
 NOW() - INTERVAL '7 days' + INTERVAL '9 hours'),

('80000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'backend', 'b0000000-0000-0000-0000-000000000002',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "backend_name": "orders-service"}'::jsonb,
 '10.0.1.50', 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2',
 NOW() - INTERVAL '7 days' + INTERVAL '9 hours 15 minutes'),

('80000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'backend', 'b0000000-0000-0000-0000-000000000004',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "backend_name": "auth-service"}'::jsonb,
 '10.0.1.50', 'b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3',
 NOW() - INTERVAL '7 days' + INTERVAL '9 hours 30 minutes'),

-- Day 6 ago: routes created
('80000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'created', 'route', 'a0000000-0000-0000-0000-000000000001',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "users-list", "endpoint": "/api/v1/users"}'::jsonb,
 '10.0.2.31', 'c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4',
 NOW() - INTERVAL '6 days' + INTERVAL '10 hours'),

('80000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'created', 'route', 'a0000000-0000-0000-0000-000000000009',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "auth-login", "endpoint": "/api/v1/auth/login"}'::jsonb,
 '10.0.2.31', 'd4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5',
 NOW() - INTERVAL '6 days' + INTERVAL '10 hours 20 minutes'),

('80000000-0000-0000-0000-000000000006', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'created', 'route', 'a0000000-0000-0000-0000-000000000015',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "ai-completions", "endpoint": "/api/v1/ai/completions"}'::jsonb,
 '10.0.2.31', 'e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6',
 NOW() - INTERVAL '6 days' + INTERVAL '11 hours'),

-- Day 5 ago: consumers and API keys
('80000000-0000-0000-0000-000000000007', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'consumer', 'c0000000-0000-0000-0000-000000000001',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "consumer_name": "mobile-app", "plan": "enterprise"}'::jsonb,
 '10.0.1.50', 'f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7',
 NOW() - INTERVAL '5 days' + INTERVAL '8 hours'),

('80000000-0000-0000-0000-000000000008', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'consumer', 'c0000000-0000-0000-0000-000000000002',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "consumer_name": "web-dashboard", "plan": "enterprise"}'::jsonb,
 '10.0.1.50', 'a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8',
 NOW() - INTERVAL '5 days' + INTERVAL '8 hours 10 minutes'),

('80000000-0000-0000-0000-000000000009', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'api_key', 'a1000000-0000-0000-0000-000000000001',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "key_prefix": "flex_ak_Xm7k", "consumer_name": "mobile-app"}'::jsonb,
 '10.0.1.50', 'b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9',
 NOW() - INTERVAL '5 days' + INTERVAL '8 hours 30 minutes'),

('80000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'api_key', 'a1000000-0000-0000-0000-000000000003',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "key_prefix": "flex_ak_Wn3t", "consumer_name": "web-dashboard"}'::jsonb,
 '10.0.1.50', 'c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0',
 NOW() - INTERVAL '5 days' + INTERVAL '8 hours 45 minutes'),

-- Day 5 ago: plugin configuration
('80000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001',
 'user', 'user-carol-sec', 'enabled', 'plugin', 'f0000000-0000-0000-0000-000000000001',
 '{"actor_email": "carol@acme.corp", "actor_name": "Carol Security", "plugin_name": "auth-jwt"}'::jsonb,
 '10.0.3.12', 'd0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1',
 NOW() - INTERVAL '5 days' + INTERVAL '14 hours'),

('80000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000001',
 'user', 'user-carol-sec', 'enabled', 'plugin', 'f0000000-0000-0000-0000-000000000002',
 '{"actor_email": "carol@acme.corp", "actor_name": "Carol Security", "plugin_name": "rate-limit"}'::jsonb,
 '10.0.3.12', 'e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2',
 NOW() - INTERVAL '5 days' + INTERVAL '14 hours 5 minutes'),

-- Day 4 ago: partner onboarding
('80000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'consumer', 'c0000000-0000-0000-0000-000000000003',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "consumer_name": "partner-integration", "partner": "TechFlow Inc"}'::jsonb,
 '10.0.1.50', 'f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3',
 NOW() - INTERVAL '4 days' + INTERVAL '11 hours'),

('80000000-0000-0000-0000-000000000014', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'api_key', 'a1000000-0000-0000-0000-000000000004',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "key_prefix": "flex_ak_Hp8v", "consumer_name": "partner-integration"}'::jsonb,
 '10.0.1.50', 'a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4',
 NOW() - INTERVAL '4 days' + INTERVAL '11 hours 15 minutes'),

('80000000-0000-0000-0000-000000000015', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'token_budget', 'e0000000-0000-0000-0000-000000000003',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "consumer_name": "partner-integration", "max_tokens": 20000000, "period": "month"}'::jsonb,
 '10.0.1.50', 'b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5',
 NOW() - INTERVAL '4 days' + INTERVAL '11 hours 30 minutes'),

-- Day 3 ago: AI model setup and webhook creation
('80000000-0000-0000-0000-000000000016', '00000000-0000-0000-0000-000000000001',
 'user', 'user-dave-ml', 'created', 'ai_model', 'd0000000-0000-0000-0000-000000000001',
 '{"actor_email": "dave@acme.corp", "actor_name": "Dave ML Engineer", "model": "openai/gpt-4o"}'::jsonb,
 '10.0.4.22', 'c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',
 NOW() - INTERVAL '3 days' + INTERVAL '9 hours'),

('80000000-0000-0000-0000-000000000017', '00000000-0000-0000-0000-000000000001',
 'user', 'user-dave-ml', 'created', 'ai_model', 'd0000000-0000-0000-0000-000000000003',
 '{"actor_email": "dave@acme.corp", "actor_name": "Dave ML Engineer", "model": "anthropic/claude-sonnet-4-20250514"}'::jsonb,
 '10.0.4.22', 'd6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7',
 NOW() - INTERVAL '3 days' + INTERVAL '9 hours 10 minutes'),

('80000000-0000-0000-0000-000000000018', '00000000-0000-0000-0000-000000000001',
 'user', 'user-dave-ml', 'enabled', 'plugin', 'f0000000-0000-0000-0000-000000000003',
 '{"actor_email": "dave@acme.corp", "actor_name": "Dave ML Engineer", "plugin_name": "ai-proxy"}'::jsonb,
 '10.0.4.22', 'e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8',
 NOW() - INTERVAL '3 days' + INTERVAL '9 hours 30 minutes'),

('80000000-0000-0000-0000-000000000019', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'webhook', '70000000-0000-0000-0000-000000000001',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "webhook_url": "https://hooks.slack.com/...", "description": "Slack notifications"}'::jsonb,
 '10.0.1.50', 'f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9',
 NOW() - INTERVAL '3 days' + INTERVAL '15 hours'),

('80000000-0000-0000-0000-000000000020', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'created', 'webhook', '70000000-0000-0000-0000-000000000002',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "webhook_url": "https://events.pagerduty.com/...", "description": "PagerDuty alerts"}'::jsonb,
 '10.0.1.50', 'a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0',
 NOW() - INTERVAL '3 days' + INTERVAL '15 hours 10 minutes'),

-- Day 2 ago: route updates, key rotation, agent activity
('80000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'updated', 'route', 'a0000000-0000-0000-0000-000000000001',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "users-list", "change": "added cors block", "version": "2 -> 3"}'::jsonb,
 '10.0.2.31', 'b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1',
 NOW() - INTERVAL '2 days' + INTERVAL '10 hours'),

('80000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'updated', 'route', 'a0000000-0000-0000-0000-000000000005',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "products-catalog", "change": "added cache_ttl 300s", "version": "3 -> 4"}'::jsonb,
 '10.0.2.31', 'c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2',
 NOW() - INTERVAL '2 days' + INTERVAL '10 hours 30 minutes'),

('80000000-0000-0000-0000-000000000023', '00000000-0000-0000-0000-000000000001',
 'user', 'user-carol-sec', 'rotated', 'api_key', 'a1000000-0000-0000-0000-000000000005',
 '{"actor_email": "carol@acme.corp", "actor_name": "Carol Security", "old_prefix": "flex_ak_Ld2m", "consumer_name": "partner-integration", "reason": "scheduled rotation"}'::jsonb,
 '10.0.3.12', 'd2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3',
 NOW() - INTERVAL '2 days' + INTERVAL '14 hours'),

('80000000-0000-0000-0000-000000000024', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000001', 'updated', 'route', 'a0000000-0000-0000-0000-000000000009',
 '{"agent_name": "deployment-bot", "route_name": "auth-login", "change": "updated rate_limit_override", "triggered_by": "github-actions/deploy-prod-#847"}'::jsonb,
 '10.0.10.5', 'e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4',
 NOW() - INTERVAL '2 days' + INTERVAL '16 hours'),

('80000000-0000-0000-0000-000000000025', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000004', 'created', 'environment', 'env-scan-report-20260320',
 '{"agent_name": "security-scanner", "scan_type": "full", "routes_scanned": 18, "issues_found": 0, "duration_ms": 4523}'::jsonb,
 '10.0.10.8', 'f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5',
 NOW() - INTERVAL '2 days' + INTERVAL '18 hours'),

-- Day 1 ago: more activity
('80000000-0000-0000-0000-000000000026', '00000000-0000-0000-0000-000000000001',
 'user', 'user-dave-ml', 'created', 'ai_model', 'd0000000-0000-0000-0000-000000000005',
 '{"actor_email": "dave@acme.corp", "actor_name": "Dave ML Engineer", "model": "google/gemini-2.0-flash", "reason": "cost optimization fallback"}'::jsonb,
 '10.0.4.22', 'a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6',
 NOW() - INTERVAL '1 day' + INTERVAL '9 hours'),

('80000000-0000-0000-0000-000000000027', '00000000-0000-0000-0000-000000000001',
 'user', 'user-dave-ml', 'updated', 'token_budget', 'e0000000-0000-0000-0000-000000000001',
 '{"actor_email": "dave@acme.corp", "actor_name": "Dave ML Engineer", "consumer_name": "mobile-app", "change": "increased max_tokens from 30M to 50M"}'::jsonb,
 '10.0.4.22', 'b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7',
 NOW() - INTERVAL '1 day' + INTERVAL '10 hours'),

('80000000-0000-0000-0000-000000000028', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000003', 'updated', 'ai_model', 'd0000000-0000-0000-0000-000000000002',
 '{"agent_name": "cost-optimizer", "model": "openai/gpt-4o-mini", "change": "increased weight from 3 to 5 based on cost efficiency analysis"}'::jsonb,
 '10.0.10.6', 'c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8',
 NOW() - INTERVAL '1 day' + INTERVAL '12 hours'),

('80000000-0000-0000-0000-000000000029', '00000000-0000-0000-0000-000000000001',
 'user', 'user-alice-admin', 'disabled', 'consumer', 'c0000000-0000-0000-0000-000000000009',
 '{"actor_email": "alice@acme.corp", "actor_name": "Alice Admin", "consumer_name": "third-party-widget", "reason": "WidgetCo contract expired"}'::jsonb,
 '10.0.1.50', 'd8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9',
 NOW() - INTERVAL '1 day' + INTERVAL '14 hours'),

('80000000-0000-0000-0000-000000000030', '00000000-0000-0000-0000-000000000001',
 'user', 'user-carol-sec', 'revoked', 'api_key', 'a1000000-0000-0000-0000-000000000011',
 '{"actor_email": "carol@acme.corp", "actor_name": "Carol Security", "key_prefix": "flex_ak_Nk1p", "consumer_name": "third-party-widget", "reason": "consumer disabled"}'::jsonb,
 '10.0.3.12', 'e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0',
 NOW() - INTERVAL '1 day' + INTERVAL '14 hours 5 minutes'),

-- Today: ongoing operations
('80000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000001',
 'system', 'system', 'created', 'environment', 'config-snapshot-20260322',
 '{"description": "Daily config snapshot", "routes_count": 18, "backends_count": 8, "consumers_count": 10}'::jsonb,
 '127.0.0.1', 'f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1',
 NOW() - INTERVAL '6 hours'),

('80000000-0000-0000-0000-000000000032', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000002', 'created', 'environment', 'health-check-20260322-0600',
 '{"agent_name": "monitoring-agent", "healthy_routes": 17, "unhealthy_routes": 1, "unhealthy": ["billing-api (inactive)"], "latency_p99_ms": 47}'::jsonb,
 '10.0.10.7', 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2',
 NOW() - INTERVAL '5 hours'),

('80000000-0000-0000-0000-000000000033', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'updated', 'route', 'a0000000-0000-0000-0000-000000000002',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "users-crud", "change": "added PATCH method support", "version": "4 -> 5"}'::jsonb,
 '10.0.2.31', 'b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3',
 NOW() - INTERVAL '4 hours'),

('80000000-0000-0000-0000-000000000034', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000001', 'updated', 'route', 'a0000000-0000-0000-0000-000000000015',
 '{"agent_name": "deployment-bot", "route_name": "ai-completions", "change": "updated timeout from 90s to 120s", "triggered_by": "github-actions/deploy-prod-#852"}'::jsonb,
 '10.0.10.5', 'c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4',
 NOW() - INTERVAL '3 hours'),

('80000000-0000-0000-0000-000000000035', '00000000-0000-0000-0000-000000000001',
 'user', 'user-eve-ops', 'created', 'api_key', 'a1000000-0000-0000-0000-000000000014',
 '{"actor_email": "eve@acme.corp", "actor_name": "Eve Ops", "key_prefix": "flex_ak_Tc2f", "consumer_name": "web-dashboard", "reason": "staging environment key"}'::jsonb,
 '10.0.5.14', 'd4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5',
 NOW() - INTERVAL '2 hours 30 minutes'),

('80000000-0000-0000-0000-000000000036', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000003', 'updated', 'token_budget', 'e0000000-0000-0000-0000-000000000004',
 '{"agent_name": "cost-optimizer", "team": "ml-platform", "change": "adjusted hourly budget based on usage patterns", "recommendation": "Consider switching default model to gpt-4o-mini"}'::jsonb,
 '10.0.10.6', 'e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6',
 NOW() - INTERVAL '2 hours'),

('80000000-0000-0000-0000-000000000037', '00000000-0000-0000-0000-000000000001',
 'user', 'user-carol-sec', 'updated', 'plugin', 'f0000000-0000-0000-0000-000000000004',
 '{"actor_email": "carol@acme.corp", "actor_name": "Carol Security", "plugin_name": "cors", "change": "added docs.acme.corp to allowed_origins"}'::jsonb,
 '10.0.3.12', 'f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7',
 NOW() - INTERVAL '1 hour 30 minutes'),

('80000000-0000-0000-0000-000000000038', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000004', 'created', 'environment', 'sec-scan-20260322-1400',
 '{"agent_name": "security-scanner", "scan_type": "incremental", "routes_scanned": 3, "issues_found": 0, "checked": ["users-crud", "ai-completions", "cors-config"]}'::jsonb,
 '10.0.10.8', 'a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8',
 NOW() - INTERVAL '1 hour'),

('80000000-0000-0000-0000-000000000039', '00000000-0000-0000-0000-000000000001',
 'user', 'user-bob-dev', 'disabled', 'route', 'a0000000-0000-0000-0000-000000000018',
 '{"actor_email": "bob@acme.corp", "actor_name": "Bob Developer", "route_name": "billing-api", "reason": "migrating to new billing service v2"}'::jsonb,
 '10.0.2.31', 'b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9',
 NOW() - INTERVAL '45 minutes'),

('80000000-0000-0000-0000-000000000040', '00000000-0000-0000-0000-000000000001',
 'agent', '90000000-0000-0000-0000-000000000002', 'created', 'environment', 'health-check-20260322-1500',
 '{"agent_name": "monitoring-agent", "healthy_routes": 17, "unhealthy_routes": 1, "unhealthy": ["billing-api (disabled)"], "latency_p99_ms": 42, "uptime_pct": 99.97}'::jsonb,
 '10.0.10.7', 'c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0',
 NOW() - INTERVAL '30 minutes')

ON CONFLICT DO NOTHING;


-- =============================================================================
-- Relationships (14 relationships connecting routes to backends, consumers to routes)
-- =============================================================================
INSERT INTO relationships (id, source_type, source_id, relationship, target_type, target_id, metadata, organization_id) VALUES
-- Routes -> Backends (proxies_to)
('50000000-0000-0000-0000-000000000001', 'route', 'a0000000-0000-0000-0000-000000000001', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000001', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000002', 'route', 'a0000000-0000-0000-0000-000000000002', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000001', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000003', 'route', 'a0000000-0000-0000-0000-000000000003', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000002', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000004', 'route', 'a0000000-0000-0000-0000-000000000007', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000003', '{"load_balance": "round_robin", "tls": true}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000005', 'route', 'a0000000-0000-0000-0000-000000000009', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000004', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000006', 'route', 'a0000000-0000-0000-0000-000000000011', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000005', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000007', 'route', 'a0000000-0000-0000-0000-000000000015', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000006', '{"load_balance": "weighted", "streaming": true}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000008', 'route', 'a0000000-0000-0000-0000-000000000012', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000007', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000009', 'route', 'a0000000-0000-0000-0000-000000000013', 'proxies_to', 'backend', 'b0000000-0000-0000-0000-000000000008', '{"load_balance": "round_robin"}'::jsonb, '00000000-0000-0000-0000-000000000001'),

-- Consumers -> Routes (consumes)
('50000000-0000-0000-0000-000000000010', 'consumer', 'c0000000-0000-0000-0000-000000000001', 'consumes', 'route', 'a0000000-0000-0000-0000-000000000001', '{"granted_at": "2026-03-15T08:00:00Z"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000011', 'consumer', 'c0000000-0000-0000-0000-000000000001', 'consumes', 'route', 'a0000000-0000-0000-0000-000000000015', '{"granted_at": "2026-03-17T11:00:00Z"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000012', 'consumer', 'c0000000-0000-0000-0000-000000000002', 'consumes', 'route', 'a0000000-0000-0000-0000-000000000001', '{"granted_at": "2026-03-15T08:00:00Z"}'::jsonb, '00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000013', 'consumer', 'c0000000-0000-0000-0000-000000000003', 'consumes', 'route', 'a0000000-0000-0000-0000-000000000005', '{"granted_at": "2026-03-18T11:00:00Z", "rate_limit": 500}'::jsonb, '00000000-0000-0000-0000-000000000001'),

-- Agent -> Routes (manages)
('50000000-0000-0000-0000-000000000014', 'agent', '90000000-0000-0000-0000-000000000001', 'manages', 'route', 'a0000000-0000-0000-0000-000000000009', '{"scope": "deployment", "auto_rollback": true}'::jsonb, '00000000-0000-0000-0000-000000000001')

ON CONFLICT DO NOTHING;
