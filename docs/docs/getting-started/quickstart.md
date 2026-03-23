---
sidebar_position: 1
title: Quickstart
description: Get FlexGateway running and proxy your first request in 5 minutes.
---

# Quickstart

This tutorial takes you from zero to a proxied request in under 5 minutes. You will start the gateway with Docker Compose, create a backend and route via the Control Plane API, and send a request through the proxy.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose (included with Docker Desktop)

That's it. No Go toolchain, no database setup.

## Step 1: Clone and start (30 seconds)

```bash
git clone https://github.com/cto-venkat/flex-gateway.git
cd flex-gateway
docker compose up -d
```

This starts three containers:

| Container | Port | Purpose |
|-----------|------|---------|
| `postgres` | 15432 | PostgreSQL 16 config store |
| `gateway` | 18080 | Data plane (proxies requests) |
| `controlplane` | 18081 | Control Plane REST API |

Wait a few seconds for all services to become healthy:

```bash
docker compose ps
```

You should see all three services with status `Up` and `(healthy)`.

## Step 2: Verify health (10 seconds)

Check that the gateway is running:

```bash
curl http://localhost:18080/healthz
```

Expected response:

```json
{"status":"ok"}
```

Check readiness (confirms database connection):

```bash
curl http://localhost:18080/readyz
```

Expected response:

```json
{"status":"ready"}
```

## Step 3: Create a backend and route (1 minute)

Use the Control Plane API on port 18081 to create a backend pointing to [httpbin.org](https://httpbin.org), a public HTTP testing service.

**Create a backend:**

```bash
curl -s -X POST http://localhost:18081/api/v1/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "httpbin-backend",
    "config": {
      "urls": ["https://httpbin.org"],
      "timeout": "10s"
    }
  }' | jq .
```

Note the `id` field in the response. You will use it in the next command.

```json
{
  "id": "a1b2c3d4-...",
  "name": "httpbin-backend",
  "config": {
    "urls": ["https://httpbin.org"],
    "timeout": "10s"
  },
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-01T00:00:00Z"
}
```

**Create a route that proxies `/demo/*` to the httpbin backend:**

Replace `BACKEND_ID` with the `id` from the previous response.

```bash
curl -s -X POST http://localhost:18081/api/v1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "name": "demo-route",
    "config": {
      "path": "/demo/*",
      "methods": ["GET"],
      "backend_id": "BACKEND_ID",
      "strip_prefix": "/demo"
    }
  }' | jq .
```

The gateway picks up the new configuration automatically via PostgreSQL LISTEN/NOTIFY. No restart needed.

## Step 4: Proxy a request (10 seconds)

Send a request through the gateway:

```bash
curl http://localhost:18080/demo/get
```

You should see the httpbin `/get` response, proxied through FlexGateway:

```json
{
  "args": {},
  "headers": {
    "Accept": "*/*",
    "Host": "httpbin.org",
    "User-Agent": "curl/8.x"
  },
  "origin": "...",
  "url": "https://httpbin.org/get"
}
```

## Step 5: Check deep health (10 seconds)

The deep health endpoint verifies all subsystems including database connectivity:

```bash
curl http://localhost:18080/healthz/deep
```

Expected response:

```json
{
  "status": "ok",
  "checks": {
    "database": {"status": "ok"}
  }
}
```

## Clean up

To stop all containers:

```bash
docker compose down
```

To also remove the data volume:

```bash
docker compose down -v
```

## Enable security checks (optional, 1 minute)

FlexGateway v0.5 includes a security block pipeline with OWASP checks, DLP, and secret scanning. Enable it on your route:

```bash
curl -s -X PATCH http://localhost:18081/api/v1/routes/ROUTE_ID \
  -H "Content-Type: application/json" \
  -d '{
    "config": {
      "blocks": [
        {
          "type": "security",
          "config": {
            "owasp_enabled": true,
            "dlp_enabled": true,
            "secret_scanning_enabled": true
          }
        }
      ]
    }
  }' | jq .
```

Requests to `/demo/*` will now pass through OWASP, DLP, and secret scanning checks before reaching the backend.

## Configure SSO (optional, 2 minutes)

If you have an OIDC provider (Zitadel, Auth0, Okta, etc.), you can enable SSO for the control plane:

```bash
curl -s -X POST http://localhost:18081/api/v1/sso/oidc \
  -H "Content-Type: application/json" \
  -d '{
    "issuer": "https://your-idp.example.com",
    "client_id": "your-client-id",
    "client_secret": "your-client-secret",
    "redirect_uri": "http://localhost:18081/auth/callback"
  }' | jq .
```

FlexGateway validates the configuration by fetching the OIDC discovery document before persisting.

## Check compliance status (optional, 30 seconds)

View your gateway's compliance posture:

```bash
curl -s http://localhost:18081/api/v1/compliance/status | jq .
```

This returns SOC 2 and GDPR control evaluation results with readiness scores.

## What's next

- [Installation Guide](./installation.md) -- building from source, Helm, environment variables
- [Configuration Guide](../guides/configuration.md) -- route matching, block chains, backends
- [AI Gateway Guide](../guides/ai-gateway.md) -- LLM proxy, multi-provider routing, token budgets
- [Plugin Tutorial](../plugin-system/plugin-tutorial.md) -- building custom blocks with the Go SDK
- [Deployment Guide](../guides/deployment.md) -- production setup, Kubernetes, Helm charts
