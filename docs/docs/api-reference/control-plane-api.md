---
sidebar_position: 1
title: Control Plane API
description: Complete REST API reference for the FlexGateway control plane.
---

# Control Plane API

The control plane exposes a REST API on the configured port (default 18081). All endpoints accept and return JSON. Authentication is required for all endpoints except health checks.

## Core Resources

### Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/routes` | List all routes |
| `POST` | `/api/v1/routes` | Create a route |
| `GET` | `/api/v1/routes/:id` | Get a route by ID |
| `PUT` | `/api/v1/routes/:id` | Update a route |
| `PATCH` | `/api/v1/routes/:id` | Partial update a route |
| `DELETE` | `/api/v1/routes/:id` | Delete a route |

### Backends

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/backends` | List all backends |
| `POST` | `/api/v1/backends` | Create a backend |
| `GET` | `/api/v1/backends/:id` | Get a backend by ID |
| `PUT` | `/api/v1/backends/:id` | Update a backend |
| `DELETE` | `/api/v1/backends/:id` | Delete a backend |

### Consumers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/consumers` | List all consumers |
| `POST` | `/api/v1/consumers` | Create a consumer |
| `GET` | `/api/v1/consumers/:id` | Get a consumer by ID |
| `PUT` | `/api/v1/consumers/:id` | Update a consumer |
| `DELETE` | `/api/v1/consumers/:id` | Delete a consumer |

### API Keys

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/consumers/:id/keys` | List keys for a consumer |
| `POST` | `/api/v1/consumers/:id/keys` | Create an API key |
| `DELETE` | `/api/v1/consumers/:id/keys/:keyId` | Revoke an API key |

### AI Models

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/ai/models` | List configured AI models |
| `POST` | `/api/v1/ai/models` | Register an AI model |
| `GET` | `/api/v1/ai/models/:id` | Get model details |
| `PUT` | `/api/v1/ai/models/:id` | Update model configuration |
| `DELETE` | `/api/v1/ai/models/:id` | Remove a model |

### Token Budgets

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/ai/budgets` | List token budgets |
| `POST` | `/api/v1/ai/budgets` | Create a token budget |
| `GET` | `/api/v1/ai/budgets/:id` | Get budget details and usage |
| `PUT` | `/api/v1/ai/budgets/:id` | Update budget limits |
| `DELETE` | `/api/v1/ai/budgets/:id` | Delete a budget |

### Plugins

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/plugins` | List installed plugins |
| `POST` | `/api/v1/plugins` | Install a plugin |
| `GET` | `/api/v1/plugins/:id` | Get plugin details |
| `PUT` | `/api/v1/plugins/:id` | Update plugin configuration |
| `DELETE` | `/api/v1/plugins/:id` | Uninstall a plugin |

---

## v0.5 Endpoints

### Security

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/v1/security/status` | Get security posture summary (OWASP, DLP, scanning status) | Bearer token |
| `GET` | `/api/v1/security/audit-log` | Query immutable audit log entries | Bearer token |
| `GET` | `/api/v1/security/audit-log/:id/verify` | Verify hash chain integrity for an audit entry | Bearer token |
| `POST` | `/api/v1/security/scan` | Trigger on-demand secret scan | Bearer token |

**GET /api/v1/security/status**

Returns current security block configuration and recent findings.

```json
{
  "owasp": { "enabled": true, "checks_passing": 9, "checks_total": 10 },
  "dlp": { "enabled": true, "patterns_loaded": 15 },
  "secret_scanning": { "enabled": true, "entropy_threshold": 4.5 },
  "audit_log": { "entries": 1284, "chain_valid": true }
}
```

### Compliance

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/v1/compliance/status` | Get compliance evaluation results (SOC 2 + GDPR) | Bearer token |
| `GET` | `/api/v1/compliance/soc2` | Get SOC 2 control status | Bearer token |
| `GET` | `/api/v1/compliance/gdpr` | Get GDPR control status | Bearer token |
| `POST` | `/api/v1/compliance/evaluate` | Trigger compliance re-evaluation | Bearer token |
| `GET` | `/api/v1/compliance/readiness` | Get readiness score (excludes not_applicable) | Bearer token |

**GET /api/v1/compliance/status**

Returns merged SOC 2 and GDPR evaluation with readiness score.

```json
{
  "frameworks": ["soc2", "gdpr"],
  "readiness_score": 0.87,
  "controls": {
    "passing": 24,
    "failing": 3,
    "not_applicable": 5,
    "total": 32
  },
  "evaluated_at": "2026-03-22T12:00:00Z"
}
```

### SSO

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `POST` | `/api/v1/sso/oidc` | Configure OIDC SSO provider | Bearer token |
| `GET` | `/api/v1/sso/oidc` | Get OIDC configuration (secrets redacted) | Bearer token |
| `DELETE` | `/api/v1/sso/oidc` | Remove OIDC configuration | Bearer token |
| `POST` | `/api/v1/sso/saml` | Configure SAML SSO provider | Bearer token |
| `GET` | `/api/v1/sso/saml` | Get SAML configuration | Bearer token |
| `DELETE` | `/api/v1/sso/saml` | Remove SAML configuration | Bearer token |

**POST /api/v1/sso/oidc**

Configures OIDC SSO. Validates by fetching the discovery document from the issuer before persisting.

```json
{
  "issuer": "https://idp.example.com",
  "client_id": "flex-gateway",
  "client_secret": "secret",
  "redirect_uri": "https://gateway.example.com/auth/callback",
  "scopes": ["openid", "profile", "email"]
}
```

### SCIM

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/v1/scim/v2/Users` | List provisioned users | SCIM Bearer token |
| `POST` | `/api/v1/scim/v2/Users` | Provision a user | SCIM Bearer token |
| `GET` | `/api/v1/scim/v2/Users/:id` | Get a user | SCIM Bearer token |
| `PUT` | `/api/v1/scim/v2/Users/:id` | Replace a user | SCIM Bearer token |
| `PATCH` | `/api/v1/scim/v2/Users/:id` | Update a user | SCIM Bearer token |
| `DELETE` | `/api/v1/scim/v2/Users/:id` | Deprovision a user | SCIM Bearer token |
| `GET` | `/api/v1/scim/v2/Groups` | List groups | SCIM Bearer token |
| `POST` | `/api/v1/scim/v2/Groups` | Create a group | SCIM Bearer token |
| `GET` | `/api/v1/scim/v2/Groups/:id` | Get a group | SCIM Bearer token |
| `PUT` | `/api/v1/scim/v2/Groups/:id` | Replace a group | SCIM Bearer token |
| `PATCH` | `/api/v1/scim/v2/Groups/:id` | Update a group | SCIM Bearer token |
| `DELETE` | `/api/v1/scim/v2/Groups/:id` | Delete a group | SCIM Bearer token |

SCIM endpoints use constant-time token comparison for authentication.

### Partners (Marketplace)

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `POST` | `/api/v1/partners/register` | Register as a partner | Bearer token |
| `GET` | `/api/v1/partners/:id` | Get partner profile | Bearer token |
| `PUT` | `/api/v1/partners/:id` | Update partner profile | Bearer token |
| `GET` | `/api/v1/partners/:id/status` | Get partner status (pending, approved, suspended) | Bearer token |

### Marketplace

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/v1/marketplace/listings` | Browse API product listings | Optional |
| `POST` | `/api/v1/marketplace/listings` | Publish an API listing | Partner token |
| `GET` | `/api/v1/marketplace/listings/:id` | Get listing details | Optional |
| `PUT` | `/api/v1/marketplace/listings/:id` | Update a listing | Partner token |
| `DELETE` | `/api/v1/marketplace/listings/:id` | Remove a listing | Partner token |
| `GET` | `/api/v1/marketplace/plugins` | Browse plugin marketplace | Optional |
| `POST` | `/api/v1/marketplace/plugins` | Submit a plugin | Partner token |
| `GET` | `/api/v1/marketplace/plugins/:id` | Get plugin details | Optional |
| `POST` | `/api/v1/marketplace/plugins/:id/install` | Install a plugin | Bearer token |
| `POST` | `/api/v1/marketplace/plugins/:id/rate` | Rate a plugin | Bearer token |

### A2A (Agent-to-Agent)

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `POST` | `/api/v1/a2a/register` | Register an agent | Bearer token |
| `GET` | `/api/v1/a2a/agents` | List registered agents | Bearer token |
| `GET` | `/api/v1/a2a/agents/:id` | Get agent details | Bearer token |
| `POST` | `/api/v1/a2a/send` | Send agent-to-agent message | Agent token |
| `GET` | `/api/v1/a2a/messages` | Get pending messages for an agent | Agent token |

A2A endpoints use per-pair token bucket rate limiting.

### Federation

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/v1/federation/clusters` | List federated clusters | Bearer token |
| `POST` | `/api/v1/federation/clusters` | Register a cluster | Bearer token |
| `GET` | `/api/v1/federation/clusters/:id` | Get cluster details and health | Bearer token |
| `DELETE` | `/api/v1/federation/clusters/:id` | Remove a cluster | Bearer token |
| `POST` | `/api/v1/federation/sync` | Trigger config sync to all clusters | Bearer token |
| `GET` | `/api/v1/federation/sync/status` | Get last sync status per cluster | Bearer token |

---

## Health Checks

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/healthz` | Liveness check (data plane) | None |
| `GET` | `/readyz` | Readiness check with database connectivity | None |
| `GET` | `/healthz/deep` | Deep health check of all subsystems | None |

## Error Format

All errors follow RFC 7807 Problem Details:

```json
{
  "type": "https://flexgateway.io/problems/not-found",
  "title": "Not Found",
  "status": 404,
  "detail": "Route with ID abc123 not found",
  "instance": "/api/v1/routes/abc123"
}
```

## Pagination

List endpoints support cursor-based pagination:

```
GET /api/v1/routes?limit=20&cursor=eyJpZCI6ImFiYzEyMyJ9
```

Response includes pagination metadata:

```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6ImRlZjQ1NiJ9",
    "has_more": true
  }
}
```
