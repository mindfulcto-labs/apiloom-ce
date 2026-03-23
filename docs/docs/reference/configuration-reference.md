---
sidebar_position: 1
title: Configuration Reference
description: Complete schema reference for routes, backends, blocks, AI models, and token budgets.
---

# Configuration Reference

This page documents the JSON schemas for all FlexGateway configuration objects. All configuration is managed through the [Control Plane API](/docs/api-reference/control-plane-api).

## Route Configuration

```json
{
  "name": "string (3-100 chars, required)",
  "config": {
    "path": "string (URL pattern, required)",
    "methods": ["GET", "POST", "..."],
    "backend_id": "UUID (required)",
    "strip_prefix": "string (optional)",
    "blocks": [
      {
        "name": "string (block identifier)",
        "config": {}
      }
    ]
  },
  "metadata": {}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Route name, 3-100 characters. Must be unique. |
| `config.path` | string | Yes | URL path pattern. Supports wildcards (`/api/*`). |
| `config.methods` | string[] | No | HTTP methods. If omitted, matches all methods. |
| `config.backend_id` | UUID | Yes | Backend to forward requests to. |
| `config.strip_prefix` | string | No | Prefix removed from path before forwarding. |
| `config.blocks` | object[] | No | Ordered block chain for this route. |
| `metadata` | object | No | Arbitrary key-value metadata. |

Routes are soft-deleted (`is_active=false`). Each update increments the `version` field and recalculates the `checksum` (SHA-256 of config).

## Backend Configuration

```json
{
  "name": "string (3-100 chars, required)",
  "config": {
    "urls": ["http://host:port"],
    "timeout": "10s"
  },
  "metadata": {}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Backend name, 3-100 characters. |
| `config.urls` | string[] | Yes | One or more upstream URLs. Multiple URLs enable round-robin load balancing. |
| `config.timeout` | string | No | Request timeout as a duration string (e.g., `"10s"`, `"30s"`). |
| `metadata` | object | No | Arbitrary metadata. |

Backends are hard-deleted (no soft delete).

## Block Configuration Schemas

### Auth (JWT)

```json
{
  "name": "auth-jwt",
  "config": {
    "jwks_url": "https://auth.example.com/.well-known/jwks.json"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `jwks_url` | string | URL of the JWKS endpoint. Keys are cached with TTL and refreshed in the background. |

Supports RS256 and ES256 algorithms. Unknown `kid` values trigger a single retry refresh of the JWKS cache. Uses server lifecycle context for cache management, not per-request context.

### Auth (API Key)

```json
{
  "name": "auth-apikey",
  "config": {}
}
```

No additional configuration needed. The block reads the `X-API-Key` header and validates against bcrypt-hashed keys in PostgreSQL. An 8-character prefix TTL cache avoids database lookups for repeated keys.

### Rate Limit

```json
{
  "name": "rate-limit",
  "config": {
    "rps": 10,
    "burst": 20,
    "window": "1m",
    "sync_interval": "250ms",
    "fail_closed": true
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rps` | int | 10 | Requests per second. |
| `burst` | int | 20 | Maximum burst size (token bucket capacity). |
| `window` | string | `"1m"` | Sliding window period for distributed counting. |
| `sync_interval` | string | `"250ms"` | How often local counts flush to PostgreSQL. |
| `fail_closed` | bool | `true` | If `true`, reject requests when the rate limit store is unavailable. If `false`, allow requests through. |

Rate limiting is dual-layer: a local token bucket handles burst, while a distributed sliding window (PostgreSQL-backed) enforces aggregate limits across gateway replicas. Counters use an `UNLOGGED` table for performance -- counter data may be lost on database crash, which is acceptable for ephemeral rate limit state.

### CORS

```json
{
  "name": "cors",
  "config": {
    "allowed_origins": ["https://app.example.com"],
    "allowed_methods": ["GET", "POST", "PUT"],
    "allowed_headers": ["Content-Type", "Authorization"],
    "max_age": 3600,
    "allow_all_origins": false
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `allowed_origins` | string[] | `[]` | Origins permitted in CORS requests. |
| `allowed_methods` | string[] | `[]` | HTTP methods permitted. |
| `allowed_headers` | string[] | `[]` | Request headers permitted. |
| `max_age` | int | `0` | Preflight cache duration in seconds. |
| `allow_all_origins` | bool | `false` | Must be explicitly `true` to allow `*`. A wildcard in `allowed_origins` is ignored unless this is set. |

### Size Limit

```json
{
  "name": "size-limit",
  "config": {
    "max_bytes": 1048576
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `max_bytes` | int | Maximum request body size in bytes. Bodies exceeding this return 413. |

Uses `io.LimitReader(maxBytes+1)` to detect oversized bodies during streaming without buffering.

### mTLS

```json
{
  "name": "mtls",
  "config": {
    "cert_file": "/certs/client.crt",
    "key_file": "/certs/client.key",
    "ca_file": "/certs/ca.crt"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `cert_file` | string | Path to client certificate. |
| `key_file` | string | Path to client private key. |
| `ca_file` | string | Path to CA certificate for server verification. |

Certificates are loaded per-connection via `GetClientCertificate` callback, supporting rotation without restart.

### Timeout

```json
{
  "name": "timeout",
  "config": {
    "timeout": "30s"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `timeout` | string | Request timeout duration. Returns 504 on deadline exceeded. |

### Circuit Breaker

```json
{
  "name": "circuit-breaker",
  "config": {
    "max_failures": 5,
    "timeout": "60s"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `max_failures` | int | Number of consecutive failures before opening the circuit. 5xx responses count as failures. |
| `timeout` | string | How long the circuit stays open before allowing a probe request. |

### Retry

```json
{
  "name": "retry",
  "config": {
    "max_retries": 3,
    "retry_budget": 0.2,
    "retry_non_idempotent": false
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_retries` | int | 3 | Maximum retry attempts. |
| `retry_budget` | float | 0.2 | Maximum fraction of requests that can be retries (prevents retry storms). |
| `retry_non_idempotent` | bool | `false` | Whether to retry POST/PATCH requests. |

Uses exponential backoff with crypto/rand jitter.

## AI Model Configuration

```json
{
  "provider": "string (required)",
  "model_name": "string (required)",
  "display_name": "string (optional)",
  "input_price_per_token": 0.000005,
  "output_price_per_token": 0.000015,
  "capabilities": ["chat", "function_calling", "vision"],
  "weight": 1
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `provider` | string | Yes | Provider identifier: `openai`, `anthropic`, or `google`. |
| `model_name` | string | Yes | Model identifier at the provider (e.g., `gpt-4o`). |
| `display_name` | string | No | Human-readable name. |
| `input_price_per_token` | float | No | Cost per input token in USD. Used for cost-optimized routing. |
| `output_price_per_token` | float | No | Cost per output token in USD. |
| `capabilities` | string[] | No | Model capabilities for capability-based routing. |
| `weight` | int | No | Load balancing weight (default 1). |

AI models are soft-deleted (`is_active=false`).

## Token Budget Configuration

```json
{
  "consumer_id": "UUID (optional)",
  "team": "string (optional)",
  "period": "month",
  "max_tokens": 1000000,
  "max_cost_usd": 100.00,
  "enforcement": "hard"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `consumer_id` | UUID | No* | Consumer this budget applies to. |
| `team` | string | No* | Team this budget applies to. |
| `period` | string | Yes | Budget period: `hour`, `day`, or `month`. |
| `max_tokens` | int | No | Token limit for the period. |
| `max_cost_usd` | float | No | Cost limit in USD for the period. |
| `enforcement` | string | No | `hard` (default) blocks requests; `soft` logs warnings. |

*At least one of `consumer_id` or `team` must be provided (enforced by database CHECK constraint).

Token budgets are hard-deleted (no soft delete).
