---
sidebar_position: 1
title: Configuration Guide
description: How FlexGateway configuration works -- routes, backends, blocks, and hot-reload.
---

# Configuration Guide

FlexGateway stores all configuration in PostgreSQL and propagates changes to the data plane via LISTEN/NOTIFY. There are no YAML config files to manage -- everything is configured through the Control Plane API.

## How Configuration Works

1. You create or update resources (routes, backends, consumers) by calling the Control Plane REST API.
2. The control plane writes the change to PostgreSQL.
3. A database trigger fires NOTIFY on the `config_changed` channel.
4. The gateway's dedicated LISTEN connection picks up the notification.
5. After a 100ms debounce window, the gateway reloads configuration from Postgres.

No restart required. No config file reload. Changes take effect in under one second.

## Routes

A route maps an incoming request path and method to a backend. Create a route via the Control Plane API:

```bash
curl -X POST http://localhost:8081/api/v1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "user-service",
    "config": {
      "path": "/api/users/*",
      "methods": ["GET", "POST", "PUT", "DELETE"],
      "backend_id": "BACKEND_UUID",
      "strip_prefix": "/api/users",
      "blocks": [
        {"name": "auth-jwt", "config": {"jwks_url": "https://auth.example.com/.well-known/jwks.json"}},
        {"name": "rate-limit", "config": {"rps": 100, "burst": 200}}
      ]
    }
  }'
```

### Route Configuration Fields

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | URL path pattern. Supports wildcards (`/api/*`) and exact matches. |
| `methods` | string[] | HTTP methods this route handles. |
| `backend_id` | UUID | The backend to forward requests to. |
| `strip_prefix` | string | Prefix to strip from the path before forwarding. |
| `blocks` | object[] | Ordered list of blocks to apply to this route. |

Routes are versioned -- each update increments the version number and generates a new config checksum.

## Backends

A backend represents an upstream service. Backends can have one or more URLs for load balancing:

```bash
curl -X POST http://localhost:8081/api/v1/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "user-backend",
    "config": {
      "urls": [
        "http://user-service-1:8080",
        "http://user-service-2:8080"
      ],
      "timeout": "10s"
    }
  }'
```

When multiple URLs are provided, the gateway distributes requests using round-robin load balancing.

### Backend Configuration Fields

| Field | Type | Description |
|-------|------|-------------|
| `urls` | string[] | One or more upstream URLs. |
| `timeout` | string | Request timeout (e.g., `"10s"`, `"30s"`). |

## Consumers and API Keys

Consumers represent API clients. Each consumer can have one or more API keys:

```bash
# Create a consumer
curl -X POST http://localhost:8081/api/v1/consumers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "acme-corp",
    "metadata": {
      "plan": "enterprise",
      "team": "backend"
    }
  }'

# Create an API key for the consumer
curl -X POST http://localhost:8081/api/v1/consumers/CONSUMER_UUID/keys
```

The API key is returned once in the creation response and cannot be retrieved again. Store it securely.

### Key Rotation

To rotate an API key without downtime:

```bash
curl -X POST http://localhost:8081/api/v1/consumers/CONSUMER_UUID/keys/KEY_UUID/rotate
```

This creates a new key and marks the old one as deprecated. The old key remains valid for a 1-hour grace period, giving you time to update clients.

### Rate Limit Tiers

Consumer metadata drives rate limit overrides. The rate limit block looks up the `plan` field in consumer metadata to determine tier-based limits:

```json
{
  "metadata": {
    "plan": "enterprise"
  }
}
```

## Block Configuration

Each block in a route's block list has a `name` and optional `config`:

```json
{
  "blocks": [
    {
      "name": "auth-jwt",
      "config": {
        "jwks_url": "https://auth.example.com/.well-known/jwks.json"
      }
    },
    {
      "name": "rate-limit",
      "config": {
        "rps": 50,
        "burst": 100,
        "window": "1m",
        "fail_closed": true
      }
    },
    {
      "name": "cors",
      "config": {
        "allowed_origins": ["https://app.example.com"],
        "allowed_methods": ["GET", "POST"],
        "max_age": 3600
      }
    }
  ]
}
```

Blocks execute in the order they appear in the list. For details on each block's configuration schema, see the [Configuration Reference](../reference/configuration-reference.md).

## AI Model Configuration

AI models are registered separately and used by the AI Proxy block:

```bash
curl -X POST http://localhost:8081/api/v1/ai/models \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "model_name": "gpt-4o",
    "display_name": "GPT-4o",
    "input_price_per_token": 0.000005,
    "output_price_per_token": 0.000015,
    "capabilities": ["chat", "function_calling", "vision"],
    "weight": 1
  }'
```

See the [AI Gateway Guide](./ai-gateway.md) for model routing, fallback chains, and token budgets.

## Next Steps

- [Environment Variables](../reference/environment-variables.md) -- all `FLEXGW_` and `FLEXCP_` variables
- [Configuration Reference](../reference/configuration-reference.md) -- complete schema for routes, backends, and blocks
- [Deployment Guide](./deployment.md) -- Docker Compose and Kubernetes setup
