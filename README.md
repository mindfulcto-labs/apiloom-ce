# Apiloom Gateway CE

High-performance, open-source API gateway built in Go. Route, authenticate, rate limit, and proxy API traffic with sub-millisecond overhead.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://go.dev)

## What is Apiloom Gateway CE?

Apiloom Gateway CE is the open-source core of the Apiloom platform. It provides a production-ready API gateway with a block-based middleware pipeline, PostgreSQL-backed config management with hot reload, and a CLI for automation.

Built on the [Lura framework](https://luraproject.org/) with a focus on performance: the auth + rate limiting pipeline adds less than 1ms overhead at 738K+ requests per second.

## Features

- **Multi-protocol routing** -- REST, GraphQL, WebSocket, gRPC, SSE through a single gateway
- **Block-based middleware** -- Composable pipeline: auth (JWT/API key), rate limiting, CORS, circuit breaker, retry, timeout, mTLS, request size limits, body transforms
- **AI gateway** -- Unified LLM proxy with model routing, token budgets, and cost ceilings
- **Hot config reload** -- PostgreSQL LISTEN/NOTIFY triggers config updates without restarts
- **Plugin SDK** -- Build custom blocks with the public Go SDK (`pkg/flex/`)
- **Observability** -- OpenTelemetry tracing, Prometheus metrics, structured JSON logging
- **CLI (fgctl)** -- Manage routes, backends, consumers, config history, and rollbacks
- **Kubernetes-ready** -- Helm charts, health probes, graceful shutdown

## Performance

Benchmarked on Apple M3 Pro:

| Scenario | Throughput | P99 Latency |
|----------|-----------|-------------|
| Auth + Rate Limit | 738,427 RPS | 0.8ms |
| Direct proxy | 1.2M+ RPS | 0.3ms |

## Quick Start

```bash
# Clone
git clone https://github.com/cto-venkat/apiloom-ce.git
cd apiloom-ce

# Start gateway + control plane + Postgres
docker-compose up -d

# Verify
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz

# Create a route via CLI
./bin/fgctl --api-url http://localhost:8081 --org <your-org-id> config history
```

## Building from Source

**Prerequisites:** Go 1.26+, PostgreSQL 16+

```bash
# Build all binaries
make build

# Or build individually
CGO_ENABLED=0 go build -o bin/apiloom-gateway ./cmd/gateway/
CGO_ENABLED=0 go build -o bin/apiloom-controlplane ./cmd/controlplane/
CGO_ENABLED=0 go build -o bin/fgctl ./cmd/fgctl/

# Run tests
make test

# Run linter
make lint

# Run benchmarks
make bench
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `FLEXGW_DATABASE_URL` | (required) | PostgreSQL connection string |
| `FLEXGW_LISTEN_ADDR` | `:8080` | Gateway listen address |
| `FLEXCP_ADDR` | `:8081` | Control plane listen address |
| `FLEXGW_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `FLEXGW_OTLP_ENDPOINT` | (empty) | OpenTelemetry collector endpoint |
| `FLEXGW_OTEL_SAMPLE_RATE` | `1.0` | Trace sampling rate |

## Architecture

```
Client -> Gateway (Data Plane) -> Backend Services
              |
              v
         Block Chain: Auth -> RateLimit -> Transform -> Proxy
              |
    Config via LISTEN/NOTIFY
              |
         Control Plane (REST API) -> PostgreSQL
              |
         CLI (fgctl)
```

The gateway uses a block chain architecture. Every feature beyond core routing is a Block implementing a simple interface. Blocks are composable: add auth, rate limiting, CORS, circuit breaking, and transforms in any order.

Config changes propagate from the control plane through PostgreSQL LISTEN/NOTIFY to the gateway, which atomically swaps its config without dropping in-flight requests.

## Plugin Development

Build custom blocks using the public SDK:

```go
import "github.com/cto-venkat/apiloom-ce/pkg/flex"

type MyBlock struct{}

func (b *MyBlock) OnRequest(ctx context.Context, req *proxy.Request) (*proxy.Request, error) {
    // Your logic here
    return req, nil
}

func (b *MyBlock) OnResponse(ctx context.Context, req *proxy.Request, resp *proxy.Response) (*proxy.Response, error) {
    return resp, nil
}
```

See `examples/plugins/` for working examples.

## Project Structure

```
cmd/gateway/         Data plane binary
cmd/controlplane/    Control plane binary
cmd/fgctl/           CLI binary
internal/gateway/    Gateway core (Lura integration, block chain, health)
internal/controlplane/ REST API handlers and stores
internal/block/      Block implementations (auth, ratelimit, cors, ...)
internal/config/     Postgres config store, LISTEN/NOTIFY, hot reload
internal/domain/     Domain types, sentinel errors
internal/observability/ OTel setup, metrics, logging
internal/plugin/     Plugin system and stores
internal/testutil/   Shared test fixtures
pkg/flex/            Public plugin SDK
api/                 Protobuf definitions
charts/              Helm charts
examples/            Example plugins and configurations
benchmarks/          Performance benchmarks
```

## Enterprise Features

For advanced capabilities like API intelligence graphs, compliance dashboards (SOC 2, GDPR, HIPAA), self-healing pipelines, golden path templates, marketplace, SSO/SCIM, and multi-cluster federation, see [Apiloom Platform](https://apiloom.io) or [request a demo](https://apiloom.io/demo.html).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
