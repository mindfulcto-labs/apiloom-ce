---
sidebar_position: 2
title: Environment Variables
description: All FLEXGW_ and FLEXCP_ environment variables for FlexGateway configuration.
---

# Environment Variables

FlexGateway is configured through environment variables prefixed with `FLEXGW_` (gateway) and `FLEXCP_` (control plane).

## Gateway Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `FLEXGW_DATABASE_URL` | PostgreSQL connection string. Format: `postgres://user:pass@host:5432/dbname?sslmode=disable` | -- | Yes |
| `FLEXGW_ADDR` | Gateway listen address. | `:8080` | No |
| `FLEXGW_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error`. | `info` | No |
| `FLEXGW_OTEL_ENDPOINT` | OpenTelemetry collector gRPC endpoint (e.g., `http://otel-collector:4317`). Enables trace and metric export when set. | -- | No |
| `FLEXGW_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout. Duration string. | `30s` | No |
| `FLEXGW_READ_TIMEOUT` | HTTP server read timeout. | `30s` | No |
| `FLEXGW_WRITE_TIMEOUT` | HTTP server write timeout. | `60s` | No |
| `FLEXGW_IDLE_TIMEOUT` | HTTP server idle connection timeout. | `120s` | No |

## Control Plane Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `FLEXCP_DSN` | PostgreSQL connection string for the control plane. Falls back to `FLEXGW_DSN` if not set. | -- | Yes (or fallback) |
| `FLEXCP_ADDR` | Control plane listen address. | `:8081` | No |
| `FLEXGW_LOG_LEVEL` | Shared log level (used by both binaries). | `info` | No |

## Provider API Keys

These are used by the AI Gateway to authenticate with LLM providers:

| Variable | Description | Required |
|----------|-------------|----------|
| `OPENAI_API_KEY` | OpenAI API key. | Only if using OpenAI models |
| `ANTHROPIC_API_KEY` | Anthropic API key. | Only if using Anthropic models |
| `GOOGLE_API_KEY` | Google Gemini API key. | Only if using Gemini models |

## PostgreSQL Connection String

The connection string follows standard PostgreSQL URI format:

```
postgres://username:password@hostname:port/database?sslmode=disable
```

### Common Parameters

| Parameter | Values | Description |
|-----------|--------|-------------|
| `sslmode` | `disable`, `require`, `verify-ca`, `verify-full` | TLS mode for database connection. Use `require` or `verify-full` in production. |
| `pool_max_conns` | integer | Maximum connections in the pool (pgx parameter). |
| `pool_min_conns` | integer | Minimum idle connections maintained. |

### Example Connection Strings

Development:
```
postgres://flexgw:flexgw_dev@localhost:5432/flexgateway?sslmode=disable
```

Production (AWS RDS):
```
postgres://flexgw:secret@mydb.xxxx.us-east-1.rds.amazonaws.com:5432/flexgateway?sslmode=verify-full
```

## Configuration Precedence

1. Environment variables take highest priority.
2. Helm chart values set environment variables in the pod spec.
3. Docker Compose environment sections set container env vars.

There are no configuration files -- all runtime configuration comes from environment variables and the PostgreSQL config store.
