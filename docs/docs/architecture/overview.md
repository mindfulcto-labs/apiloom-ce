---
sidebar_position: 1
title: Architecture Overview
description: How FlexGateway is structured -- data plane, control plane, and configuration propagation.
---

# Architecture Overview

FlexGateway is split into two binaries: a **data plane** that processes API requests and a **control plane** that manages configuration. PostgreSQL is the only external dependency.

## System Diagram

```
                     +------------------------------------------+
                     |            CONTROL PLANE                  |
                     |                                           |
  Dashboard -------->|  +----------+   +-----------------+      |
  (Next.js)          |  | REST API |   | K8s Operator    |      |
                     |  | (Go)     |   | (CRD watcher)   |      |
  CLI (fgctl) ------>|  +----+-----+   +--------+--------+      |
                     |       +--------+---------+               |
                     |           +----v-----+                   |
                     |           | Postgres  |                   |
                     |           +----+-----+                   |
                     +----------------+-------------------------+
                              LISTEN/ | NOTIFY
                     +----------------+-------------------------+
                     |           DATA PLANE                      |
                     |   +-------------------------------+       |
                     |   |   Lura-based Gateway Core     |       |
                     |   |                               |       |
                     |   |  +------+  +-------+  +-----+|       |
                     |   |  |Router|->| Block |->|Proxy||       |
                     |   |  |      |  | Chain |  |     ||       |
                     |   |  +------+  +-------+  +-----+|       |
                     |   |                               |       |
                     |   |  Blocks: Auth | RateLimit | AI|       |
                     |   |  Proxy | CORS | Transform     |       |
                     |   +-------------------------------+       |
                     |   +-------------------------------+       |
                     |   |   OpenTelemetry Exporter       |       |
                     |   +-------------------------------+       |
                     +-------------------------------------------+
```

## Component Responsibilities

| Component | Binary | Responsibility |
|-----------|--------|---------------|
| Gateway Core | `flexgateway` | HTTP routing, block chain execution, backend proxying, SSE streaming |
| Control Plane | `flexcontrolplane` | CRUD for routes, backends, consumers, plugins, AI models, token budgets |
| CLI | `fgctl` | Command-line management and scripting |
| K8s Operator | Embedded in control plane | Watch Gateway API CRDs, sync to Postgres |

## Request Processing Flow

When a request arrives at the gateway:

1. **Router** matches the request path and method to a configured route.
2. **Block chain** executes in order. Each block can inspect, modify, or reject the request. Typical order: Auth, RateLimit, CORS, SizeLimit, then any custom blocks.
3. **Proxy** forwards the request to the upstream backend. If the route has multiple backends, round-robin load balancing distributes traffic.
4. **Response** passes back through any response-phase block processing and is returned to the client.

The data plane never queries PostgreSQL on the request hot path. All configuration is loaded into memory at startup and updated via LISTEN/NOTIFY.

## Configuration Propagation

Configuration changes reach the data plane through PostgreSQL's LISTEN/NOTIFY mechanism:

```
Dashboard   -> REST API     -> Postgres -> NOTIFY -> Data Plane reload
CLI         -> REST API     -> Postgres -> NOTIFY -> Data Plane reload
CRD change  -> K8s Operator -> Postgres -> NOTIFY -> Data Plane reload
```

The flow:

1. An operator creates or updates a route through the Control Plane API (or via CRD).
2. The control plane writes to PostgreSQL.
3. A PostgreSQL trigger fires NOTIFY on the `config_changed` channel.
4. The gateway's dedicated LISTEN connection (separate from the connection pool) receives the notification.
5. After a 100ms debounce window (to coalesce rapid changes), the gateway reloads its configuration from Postgres.
6. The new configuration takes effect with zero dropped requests.

## Key Design Decisions

**Lura framework as data plane foundation.** Lura (formerly KrakenD framework) provides a battle-tested HTTP proxy layer with middleware composition. FlexGateway extends it with the Block system and PostgreSQL-backed configuration.

**PostgreSQL as the only external dependency.** Config storage, rate limiting counters, AI token budgets, and usage analytics all live in Postgres. This eliminates the need for Redis, etcd, or Cassandra that other gateways require.

**Block interface for all features.** Everything beyond core routing is a Block implementing a two-method interface (`Name()` + `Middleware()`). This keeps the gateway kernel minimal and makes features hot-swappable. See [Block System](./block-system.md) for details.

**LISTEN/NOTIFY for config propagation.** Instead of polling, the gateway uses a dedicated PostgreSQL connection to receive push notifications when configuration changes. Reloads complete in under one second.

## Directory Structure

```
cmd/gateway/           -- data plane binary
cmd/controlplane/      -- control plane binary
cmd/fgctl/             -- CLI binary
internal/gateway/      -- data plane internals (Lura integration, block chain)
internal/controlplane/ -- control plane internals (REST API, config mgmt)
internal/block/        -- block implementations (auth, ratelimit, aiproxy, cors...)
internal/config/       -- Postgres config store, LISTEN/NOTIFY, hot reload
internal/plugin/       -- plugin SDK, gRPC plugin host, lifecycle management
internal/ai/           -- LLM provider adapters, token counting, budget enforcement
internal/domain/       -- domain types, sentinel errors
internal/observability/ -- OTel setup, metrics, logging
pkg/flex/              -- public plugin SDK (the only exported package)
```

## v0.5 Components

FlexGateway v0.5 adds several subsystems on top of the core gateway architecture.

### Security Block Pipeline

The security block is a composite block that chains multiple security checks in a single block chain position:

```
Request -> [OWASP Checks] -> [DLP / PII Masking] -> [Secret Scanning] -> next block
```

- **OWASP checks:** Automated checks for the OWASP API Security Top 10 (broken auth, injection, excessive data exposure, etc.) using Go regexp pattern matching.
- **DLP / PII masking:** Presidio-style PII detection with configurable masking strategies (redact, hash, mask). Fields are identified via regex and context rules.
- **Secret scanning:** Entropy analysis (Shannon entropy at 4.5 threshold) combined with pattern matching to detect secrets in request/response bodies and headers.
- **Immutable audit log:** Append-only log with SHA-256 cryptographic hash chain. Each entry includes a hash of the previous entry, creating a tamper-evident chain. UUIDs generated in Go before INSERT.

### Compliance Framework

The compliance subsystem evaluates infrastructure against regulatory frameworks:

- **SOC 2 Type I controls:** SystemProber interface evaluates controls against live infrastructure. Probers are injected for testability.
- **GDPR:** Data residency routing per region, right-to-erasure with ontology graph-powered cascade discovery (uses `GetNeighbors` for related data). Audit entries are anonymized, not deleted.
- **Envelope encryption:** Secrets at rest are encrypted using envelope encryption. LocalKEKProvider validates 32-byte KEK size; KEKProvider interface supports external KMS integration.
- **Compliance dashboard:** MultiEvaluator merges SOC 2 and GDPR into a single report. In-memory TTL cache for evaluation results. Readiness score excludes `not_applicable` controls.

### Enterprise Identity

- **SSO:** OIDC and SAML authentication via Zitadel integration. Configuration validated via live OIDC discovery or SAML metadata fetch before persisting. Uses `lestrrat-go/jwx/v2` for JWKS caching.
- **SCIM 2.0:** User and group provisioning server with store interfaces for testability. Constant-time token comparison for authentication.
- **IP allowlisting:** Per-organization IP allowlists. Private ranges rejected at configuration time (SSRF prevention).
- **MFA enforcement:** Per-organization MFA policy. Checks boolean `mfa_verified` claim; string values rejected.

### Marketplace and Partner Portal

- **Partner registration:** Separate marketplace domain package. Partner status transitions enforced at domain level.
- **API product listings:** Partners publish APIs with metadata, pricing, and documentation.
- **Plugin marketplace:** Browse, install, rate, and purchase plugins. Sandbox uses SandboxExecutor injection for testability. Plugin status transitions mirror listing lifecycle.
- **Revenue sharing:** Configurable revenue split with Stripe Connect for partner payouts.

### Multi-Cluster Federation

```
                    +-------------------+
                    |  Control Plane    |
                    |  (ClusterManager) |
                    +--------+----------+
                             |
              +--------------+--------------+
              |              |              |
         +----v----+   +----v----+   +----v----+
         | Cluster |   | Cluster |   | Cluster |
         |   A     |   |   B     |   |   C     |
         +---------+   +---------+   +---------+
```

- **ClusterManager:** In-memory cluster registry with health monitoring.
- **ConfigSyncer:** Parallel configuration push to all clusters via errgroup.
- **A2A protocol:** Agent-to-agent communication with per-pair token bucket rate limiting.

## Next Steps

- [Block System](./block-system.md) -- how blocks compose and the built-in block catalog
- [Configuration Guide](../guides/configuration.md) -- routes, backends, and block configuration
- [Deployment Guide](../guides/deployment.md) -- Docker Compose, Kubernetes, and production setup
