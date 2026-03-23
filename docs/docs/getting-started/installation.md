---
sidebar_position: 2
title: Installation
description: Install FlexGateway via Docker Compose, from source, or with Helm on Kubernetes.
---

# Installation

FlexGateway runs as two Go binaries (data plane and control plane) backed by PostgreSQL. Pick the installation method that fits your environment.

## Docker Compose (recommended for development)

The quickest path. Starts the gateway, control plane, and PostgreSQL in containers.

```bash
git clone https://github.com/cto-venkat/flex-gateway.git
cd flex-gateway
docker compose up -d
```

| Service | Host Port | Container Port |
|---------|-----------|----------------|
| Gateway (data plane) | 18080 | 8080 |
| Control Plane API | 18081 | 8081 |
| PostgreSQL | 15432 | 5432 |

See the [Quickstart](./quickstart.md) for a full walkthrough.

## Building from source

### Prerequisites

- Go 1.25 or later
- PostgreSQL 14 or later (running and accessible)
- Make

### Build

```bash
git clone https://github.com/cto-venkat/flex-gateway.git
cd flex-gateway
make build
```

This produces binaries in `bin/`:

```
bin/flexgateway       # data plane
bin/flexcontrolplane  # control plane API
```

### Run

Set the database connection string and start each binary:

```bash
# Start the data plane
export FLEXGW_DATABASE_URL="postgres://flexgw:flexgw_dev@localhost:15432/flexgateway?sslmode=disable"
./bin/flexgateway

# In another terminal, start the control plane
export FLEXGW_DATABASE_URL="postgres://flexgw:flexgw_dev@localhost:15432/flexgateway?sslmode=disable"
./bin/flexcontrolplane
```

### Run tests

```bash
make test               # unit tests with race detector
make test-integration   # integration tests (requires Docker for testcontainers)
make lint               # golangci-lint
make all                # lint + test + build
```

## Helm (Kubernetes)

FlexGateway ships with a Helm chart in `charts/flexgateway/`.

```bash
helm install flexgateway charts/flexgateway \
  --set postgresql.auth.password=your-password \
  --set config.databaseURL="postgres://user:pass@host:5432/flexgateway"
```

The chart includes:

- Gateway Deployment with health checks and resource limits
- Control Plane Deployment
- PostgreSQL dependency (optional -- bring your own)
- Service and Ingress templates
- Init container that waits for PostgreSQL readiness via `pg_isready`

See the [Deployment Guide](../guides/deployment.md) for production Helm configuration.

## Container images

Build production container images with:

```bash
make docker
```

Images use `distroless` base (`gcr.io/distroless/base-debian12:nonroot`) for a minimal attack surface. The final image is under 50MB.

## Environment variables

All FlexGateway configuration uses the `FLEXGW_` prefix.

| Variable | Default | Description |
|----------|---------|-------------|
| `FLEXGW_DATABASE_URL` | (required) | PostgreSQL connection string |
| `FLEXGW_ADDR` | `:8080` | Data plane listen address |
| `FLEXGW_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `FLEXGW_TLS_CERT` | | Path to TLS certificate |
| `FLEXGW_TLS_KEY` | | Path to TLS private key |
| `FLEXCP_ADDR` | `:8081` | Control plane listen address |
| `FLEXCP_DSN` | (falls back to `FLEXGW_DSN`) | Control plane database connection |

The control plane binary reads `FLEXCP_DSN` first, falling back to `FLEXGW_DSN` if not set. This allows running both binaries against the same database with a single environment variable.

See the [Environment Variables Reference](../reference/environment-variables.md) for the complete list.
