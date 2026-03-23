---
sidebar_position: 2
title: Deployment Guide
description: Deploy FlexGateway with Docker Compose, Kubernetes/Helm, and production considerations.
---

# Deployment Guide

FlexGateway ships as multi-arch container images built on a distroless base. The only external dependency is PostgreSQL 16+.

## Docker Compose

The repository includes a `docker-compose.yml` that starts PostgreSQL, the gateway, and the control plane:

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

### Environment Variables

The Docker Compose file uses these defaults:

```yaml
# PostgreSQL
POSTGRES_DB: flexgateway
POSTGRES_USER: flexgw
POSTGRES_PASSWORD: flexgw_dev

# Gateway
FLEXGW_DATABASE_URL: "postgres://flexgw:flexgw_dev@postgres:5432/flexgateway?sslmode=disable"
FLEXGW_LOG_LEVEL: debug

# Control Plane (same DSN by default)
FLEXGW_DATABASE_URL: "postgres://flexgw:flexgw_dev@postgres:5432/flexgateway?sslmode=disable"
```

For a full list of environment variables, see the [Environment Variables Reference](../reference/environment-variables.md).

## Kubernetes with Helm

FlexGateway includes a Helm chart at `charts/flexgateway/`.

### Install

```bash
helm install flexgw ./charts/flexgateway \
  --set postgresql.auth.password=your-secure-password \
  --namespace flexgateway \
  --create-namespace
```

### Key Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `2` | Number of gateway pods |
| `image.repository` | `flexgateway/flexgateway` | Container image |
| `image.tag` | `latest` | Image tag |
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `8080` | Service port |
| `postgresql.enabled` | `true` | Deploy internal PostgreSQL StatefulSet |
| `postgresql.external.dsn` | `""` | Use external Postgres instead (disables internal) |
| `gateway.logLevel` | `info` | Gateway log level |
| `gateway.shutdownTimeout` | `30s` | Graceful shutdown timeout |
| `gateway.readTimeout` | `30s` | HTTP read timeout |
| `gateway.writeTimeout` | `60s` | HTTP write timeout |
| `podDisruptionBudget.enabled` | `true` | Enable PDB (minAvailable: 1) |

### Using an External Database

To connect to a managed PostgreSQL instance (RDS, Cloud SQL, etc.):

```bash
helm install flexgw ./charts/flexgateway \
  --set postgresql.enabled=false \
  --set postgresql.external.dsn="postgres://user:pass@rds-host:5432/flexgateway?sslmode=require"
```

### Resource Limits

Default resource requests and limits:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

Adjust based on traffic volume. For high-throughput deployments (10K+ RPS), increase CPU limits to 2-4 cores and memory to 512Mi-1Gi.

## Container Image

The production image uses `gcr.io/distroless/base-debian12:nonroot`:

- No shell, no package manager -- reduced attack surface
- Runs as non-root user (UID 65532)
- Read-only root filesystem
- Multi-arch: linux/amd64 and linux/arm64
- Includes CA certificates (needed for HTTPS upstream connections)

## Production Considerations

### Database Connection Pooling

The gateway uses pgx with a connection pool. Default pool size is suitable for development. For production:

- Set `FLEXGW_DATABASE_URL` with appropriate pool parameters
- Use a connection pooler like PgBouncer if running many gateway replicas
- The LISTEN/NOTIFY connection is separate from the pool and uses a dedicated `pgx.Conn`

### Replica Count

The gateway is stateless (all state lives in PostgreSQL), so horizontal scaling is straightforward. Each replica receives NOTIFY events independently and reloads its own configuration.

Recommended minimums:
- **Development:** 1 replica
- **Staging:** 2 replicas
- **Production:** 3+ replicas behind a load balancer

### Health Probes

Configure Kubernetes probes to use the built-in health endpoints:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

The `/healthz` endpoint returns `{"status":"ok"}`. The `/readyz` endpoint also verifies database connectivity.

### Graceful Shutdown

The gateway handles SIGTERM by:

1. Stopping acceptance of new connections.
2. Waiting for in-flight requests to complete (up to `gateway.shutdownTimeout`).
3. Closing the LISTEN/NOTIFY connection.
4. Closing the database pool.

### Security Context

The Helm chart sets a restrictive security context by default:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  fsGroup: 65532

securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
```

### Topology Spread

The chart supports topology spread constraints and pod anti-affinity to distribute pods across nodes:

```yaml
topologySpreadConstraints:
  enabled: true
  maxSkew: 1
  topologyKey: kubernetes.io/hostname

podAntiAffinity:
  enabled: true
```

## Monitoring

### Prometheus

The gateway exposes a Prometheus metrics endpoint. Scrape configuration:

```yaml
scrape_configs:
  - job_name: flexgateway
    static_configs:
      - targets: ['flexgateway:8080']
    metrics_path: /metrics
```

Exported metrics include request latency histograms (5ms to 10s buckets), request counts by route and status code, and active connection gauges.

### OpenTelemetry

Set `FLEXGW_OTEL_ENDPOINT` to export traces and metrics to an OpenTelemetry collector:

```bash
FLEXGW_OTEL_ENDPOINT=http://otel-collector:4317
```

Trace context propagates through the full request lifecycle: router, block chain, proxy, and backend call.

## Next Steps

- [Quickstart](../getting-started/quickstart.md) -- get running in 5 minutes
- [Configuration Guide](./configuration.md) -- routes, backends, and blocks
- [Environment Variables](../reference/environment-variables.md) -- all configuration options
