# Apiloom Gateway CE

High-performance, open-source API gateway built in Go. Route, authenticate, rate limit, and proxy API traffic with sub-millisecond overhead.

[![CI](https://github.com/cto-venkat/apiloom-ce/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/cto-venkat/apiloom-ce/actions/workflows/ci.yml)
[![Security](https://github.com/cto-venkat/apiloom-ce/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/cto-venkat/apiloom-ce/actions/workflows/security.yml)
[![Release](https://img.shields.io/github/v/release/cto-venkat/apiloom-ce?include_prereleases&sort=semver)](https://github.com/cto-venkat/apiloom-ce/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/cto-venkat/apiloom-ce/pkg/flex.svg)](https://pkg.go.dev/github.com/cto-venkat/apiloom-ce/pkg/flex)

## What is Apiloom Gateway CE?

Apiloom Gateway CE is an open-source API gateway you can run in front of your services. It does the standard gateway job — routing, authentication, rate limiting, retries, circuit breaking, observability — and adds a few things that are usually pay-walled elsewhere: a unified LLM proxy, hot configuration reload via PostgreSQL `LISTEN/NOTIFY`, and a public Go plugin SDK.

It is the data-plane core of the [Apiloom Platform](https://apiloom.io). The CE distribution is Apache 2.0 and ships with everything you need to run it in production: a CLI, a Helm chart, container images, signed releases, and a benchmark harness you can rerun on your own hardware.

It is built on top of the [Lura framework](https://luraproject.org/), which we extended with a block-chain middleware model so each request feature (auth, rate limit, transform, proxy) is composable and independently testable.

## Performance

Benchmarked on Apple M3 Pro, eight cores, Postgres 16. Reproduce with `make bench` or the harness in [`benchmarks/`](benchmarks/).

| Scenario | Throughput | P99 latency |
|---|---|---|
| Auth + rate limit + proxy | 738,427 RPS | 0.8 ms |
| Direct proxy (no blocks) | 1.2M+ RPS | 0.3 ms |

These numbers are not marketing artifacts. The harness, the test plan, and the raw output are in the repository.

## Features

- **Multi-protocol routing** — REST, GraphQL, WebSocket, gRPC, and SSE through a single gateway.
- **Block-based middleware** — auth (JWT, API key, mTLS), rate limiting (local and distributed), CORS, circuit breaker, retry, timeout, request size limits, body transforms.
- **Unified LLM proxy** — model routing across OpenAI, Anthropic, and Gemini with token counting and per-tenant cost ceilings.
- **Hot config reload** — PostgreSQL `LISTEN/NOTIFY` propagates config changes to running gateways with no restart and no in-flight request loss.
- **Plugin SDK** — build custom blocks against the public Go SDK ([`pkg/flex/`](pkg/flex/)).
- **Observability** — OpenTelemetry tracing, Prometheus metrics, structured JSON logging.
- **CLI (`fgctl`)** — manage routes, backends, consumers, config history, and rollbacks.
- **Kubernetes-ready** — Helm chart, health and readiness probes, graceful shutdown.

## Quick start

Bring up the full stack with Docker Compose. Five minutes from clone to a proxied request.

```bash
git clone https://github.com/cto-venkat/apiloom-ce.git
cd apiloom-ce

# Start gateway + control plane + Postgres
docker compose up -d

# Verify both planes are healthy
curl http://localhost:18080/healthz   # data plane
curl http://localhost:18081/healthz   # control plane
```

Both endpoints return `{"status":"ok"}`.

For the full quickstart — creating a backend, adding a route, sending a proxied request, enabling the security and SSO blocks — follow [`docs/docs/getting-started/quickstart.md`](docs/docs/getting-started/quickstart.md).

## Building from source

**Prerequisites:** Go 1.25+, PostgreSQL 16+ (or Docker).

```bash
make build               # all binaries
make test                # unit tests with race detector
make test-integration    # integration tests (needs Postgres)
make lint                # golangci-lint + go vet
make bench               # benchmark suite
make security            # govulncheck + gosec
make all                 # lint + test + build
```

Individual binaries:

```bash
CGO_ENABLED=0 go build -o bin/apiloom-gateway       ./cmd/gateway/
CGO_ENABLED=0 go build -o bin/apiloom-controlplane  ./cmd/controlplane/
CGO_ENABLED=0 go build -o bin/fgctl                 ./cmd/fgctl/
```

## Configuration

All configuration is via environment variables. Full list in [`docs/docs/reference/environment-variables.md`](docs/docs/reference/environment-variables.md).

| Variable | Default | Description |
|---|---|---|
| `FLEXGW_DATABASE_URL` | (required) | PostgreSQL connection string |
| `FLEXGW_LISTEN_ADDR` | `:8080` | Gateway listen address |
| `FLEXCP_ADDR` | `:8081` | Control plane listen address |
| `FLEXGW_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `FLEXGW_OTEL_ENDPOINT` | (empty) | OpenTelemetry collector endpoint |
| `FLEXGW_OTEL_SAMPLE_RATE` | `1.0` | Trace sampling rate |

> Environment variable names are still on the `FLEXGW_*` / `FLEXCP_*` prefix from the project's earlier name. They will be aliased to `APILOOM_*` in a future minor release with backward compatibility.

## Architecture

```
                  ┌────────────────────────────┐
                  │        Control Plane       │
                  │  REST API ─ Postgres store │
                  └────────────┬───────────────┘
                               │ LISTEN / NOTIFY
                               ▼
 Client ──► Gateway (Data Plane) ──► Backend Services
            │
            └─ Block chain: auth ─► rate limit ─► transform ─► proxy
```

Every gateway feature beyond core routing is a `Block` implementing a small interface. Blocks compose freely — add auth, rate limiting, CORS, circuit breaking, and transforms in any order. Config changes flow from the control plane through Postgres `LISTEN/NOTIFY` to running gateway processes, which atomically swap their config without dropping in-flight requests.

For a deeper architecture treatment, see:

- [Architecture overview](docs/docs/architecture/overview.md)
- [Data flow](docs/docs/architecture/data-flow.md)
- [Block system](docs/docs/architecture/block-system.md)

## Plugin development

Build custom blocks against the public SDK. Example:

```go
package main

import (
    "context"

    "github.com/cto-venkat/apiloom-ce/pkg/flex"
    "github.com/luraproject/lura/v2/proxy"
)

type AddHeader struct{ Name, Value string }

func (h *AddHeader) OnRequest(ctx context.Context, req *proxy.Request) (*proxy.Request, error) {
    req.Headers[h.Name] = []string{h.Value}
    return req, nil
}

func (h *AddHeader) OnResponse(_ context.Context, _ *proxy.Request, resp *proxy.Response) (*proxy.Response, error) {
    return resp, nil
}

var _ flex.Block = (*AddHeader)(nil)
```

Working examples are in [`examples/plugins/`](examples/plugins/) (custom auth, response transform, webhook). The full plugin guide is at [`docs/docs/plugin-system/plugin-guide.md`](docs/docs/plugin-system/plugin-guide.md).

## Documentation

- **Get started:** [Quickstart](docs/docs/getting-started/quickstart.md) · [Installation](docs/docs/getting-started/installation.md)
- **Architecture:** [Overview](docs/docs/architecture/overview.md) · [Data flow](docs/docs/architecture/data-flow.md) · [Block system](docs/docs/architecture/block-system.md)
- **Reference:** [API (OpenAPI)](docs/docs/api-reference/openapi.md) · [Control Plane API](docs/docs/api-reference/control-plane-api.md) · [Configuration](docs/docs/reference/configuration-reference.md) · [Environment variables](docs/docs/reference/environment-variables.md)
- **Guides:** [Configuration](docs/docs/guides/configuration.md) · [Deployment](docs/docs/guides/deployment.md) · [AI gateway](docs/docs/guides/ai-gateway.md) · [GitOps](docs/docs/gitops-workflow.md)
- **Plugins:** [Tutorial](docs/docs/plugin-system/plugin-tutorial.md) · [Guide](docs/docs/plugin-system/plugin-guide.md) · [Examples](examples/plugins/)
- **Project:** [Changelog](CHANGELOG.md) · [Governance](GOVERNANCE.md) · [Maintainers](MAINTAINERS.md) · [Maintainer ladder](docs/maintainer-ladder.md) · [Release process](docs/release-process.md) · [Security policy](docs/SECURITY.md) · [Adopters](ADOPTERS.md)

## Project structure

```
cmd/gateway/            data plane binary
cmd/controlplane/       control plane binary
cmd/fgctl/              CLI binary
internal/gateway/       gateway core (Lura integration, block chain, health)
internal/controlplane/  REST API handlers and stores
internal/block/         block implementations (auth, ratelimit, cors, …)
internal/config/        Postgres config store, LISTEN/NOTIFY, hot reload
internal/domain/        domain types, sentinel errors
internal/observability/ OTel setup, metrics, logging
internal/plugin/        plugin system and stores
internal/testutil/      shared test fixtures
pkg/flex/               public plugin SDK
api/                    protobuf definitions
charts/                 Helm charts
examples/               example plugins and configurations
benchmarks/             performance benchmarks
```

## Adopters

Are you running Apiloom Gateway CE? Add yourself to [`ADOPTERS.md`](ADOPTERS.md) — even a one-line entry helps the project and the maturity case for CNCF.

## Community

- **GitHub Discussions** — [github.com/cto-venkat/apiloom-ce/discussions](https://github.com/cto-venkat/apiloom-ce/discussions)
- **Issues** — [github.com/cto-venkat/apiloom-ce/issues](https://github.com/cto-venkat/apiloom-ce/issues) · check the `good first issue` label if you want to contribute
- **Discord** — coming soon. Watch the [latest release](https://github.com/cto-venkat/apiloom-ce/releases) for the invite link
- **Security reports** — `security@apiloom.io` (do not file public issues; see [`docs/SECURITY.md`](docs/SECURITY.md))
- **Code of Conduct** — [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) for development setup, the test workflow, and the PR process. New contributors should look at issues labelled `good first issue`.

## Enterprise features

Apiloom Gateway CE is the open-source core. The [Apiloom Platform](https://apiloom.io) builds on it with managed control plane, API intelligence graphs, compliance dashboards (SOC 2, GDPR, HIPAA), self-healing pipelines, golden-path templates, a partner marketplace, SSO/SCIM, and multi-cluster federation. [Request a demo](https://apiloom.io/demo.html).

## License

[Apache License 2.0](LICENSE).
