# Apiloom Gateway CE — Benchmarks

Reproducible performance testing infrastructure for Apiloom Gateway CE. Includes Go microbenchmarks for hot-path functions and k6 load tests for end-to-end throughput measurement.

**Canonical baseline results and methodology:** see [`RESULTS.md`](RESULTS.md).

## Reference Environment

The baseline numbers in `RESULTS.md` were produced on:

| Field | Value |
|-------|-------|
| **Machine** | Apple M2 Max (MacBook Pro 14-inch, 2023) |
| **CPU** | 12-core (8P + 4E), 3.5 GHz peak |
| **RAM** | 32 GB |
| **OS** | macOS Sequoia 15.4, Darwin 25.4.0 arm64 |
| **Go version** | go1.26.x |
| **GOMAXPROCS** | 12 |
| **k6 version** | 0.57.x (for load tests) |
| **k6 VUs (full run)** | 1000 VUs, 4-minute staged profile |
| **k6 VUs (CI smoke)** | 20 VUs, 60-second flat run |
| **Gateway config** | Default, no custom tuning |
| **PostgreSQL** | 16 (not exercised in microbenchmarks) |

Numbers from other environments will differ. Always record your hardware when sharing results
(see §Hardware Disclosure below).

## Performance Targets

| Pipeline | Target RPS | Target P99 | Benchmark |
|----------|-----------|-----------|-----------|
| Auth + rate-limit (Lura proxy layer) | 738K | < 1ms | `BenchmarkAuthRateLimit` |
| Proxy passthrough (Lura proxy layer) | 500K | < 2ms | `BenchmarkProxyPassthrough` |
| AI proxy (token counting + budget) | 80K | < 5ms | `BenchmarkAIProxy` |
| ProxyHandler end-to-end (HTTP) | — | < 50µs | `BenchmarkProxyHandler` |

These targets apply to the **Lura proxy layer** (in-memory mock backend, no OS network stack).
End-to-end k6 numbers will be lower and are separately tracked in `RESULTS.md §4`.

## Prerequisites

- **Go 1.25+** -- [install](https://go.dev/doc/install)
- **Docker and Docker Compose** -- [install](https://docs.docker.com/get-docker/)
- **k6** -- [install](https://grafana.com/docs/k6/latest/set-up/install-k6/)
  - macOS: `brew install k6`
  - Linux: see the k6 installation docs linked above

## Quick Start

### 1. Run Go Microbenchmarks

These test individual components (proxy handler, middleware chain, route matching) without requiring a running gateway or database.

```bash
cd benchmarks
make bench-go
```

Output goes to `results/go-bench.txt`. Use `benchstat` to compare across runs:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Compare two runs
benchstat results/go-bench-baseline.txt results/go-bench.txt
```

### 2. Run k6 Load Tests

Load tests require a running gateway with a mock backend.

```bash
# Start the gateway and mock backend
docker compose -f docker-compose.yml -f benchmarks/docker-compose.bench.yml up -d

# Run the full staged load test (ramp to 1000 VUs over 4 minutes)
cd benchmarks
make bench-k6

# Or run a quick smoke test (100 VUs for 30 seconds)
make bench-k6-smoke
```

The load test targets `http://localhost:18080` by default. Override with:

```bash
make bench-k6 GATEWAY_URL=http://your-gateway:8080
```

### 3. Run Everything

```bash
cd benchmarks
make bench-all
```

## What Each Benchmark Measures

### Go Microbenchmarks (`internal/gateway/bench_test.go`)

| Benchmark | What it measures |
|---|---|
| `BenchmarkProxyHandler` | Full HTTP request through proxy handler with mock backend. Uses `b.RunParallel` for concurrent throughput. |
| `BenchmarkBlockChain` | Request through 3-block middleware chain. Measures composition overhead. |
| `BenchmarkRouteMatch` | Chi router lookup against 100 routes. Measures routing table scan cost. |
| `BenchmarkJSONResponse` | JSON marshaling of a typical API response. Measures serialization overhead. |

### k6 Load Test (`k6/load-test.js`)

| Scenario | Weight | What it measures |
|---|---|---|
| GET /healthz | 20% | Gateway overhead without proxying |
| GET /demo/get | 40% | End-to-end proxied GET latency |
| POST /demo/post | 40% | Proxied POST with JSON body |

## Docker Compose Benchmark Environment

The `docker-compose.bench.yml` override adds a mock backend (nginx returning static JSON) so load tests do not depend on external services. The nginx config is in `nginx-bench.conf`.

## Interpreting Results

- **ns/op below 10,000:** Per-request overhead is under 10 microseconds. Typical for Go HTTP handlers.
- **B/op and allocs/op:** Track these across releases. A jump in allocations often signals a performance regression.
- **k6 p99 < 50ms:** End-to-end latency target. If p99 exceeds this under load, investigate with `go tool pprof`.
- **Error rate < 1%:** Any higher suggests resource exhaustion or bugs under concurrency.
- **CI regression gate:** The `benchmark.yml` workflow fails if any benchmark degrades by more than 10%
  compared to the `main` branch baseline (measured via `benchstat` geomean). P99 > 1.5ms at the proxy
  layer also fails the gate.

## Comparison with Other Gateways

The nginx mock backend in `nginx-bench.conf` serves static JSON with zero application logic, giving
a raw OS + network stack baseline. The table below uses published data from each gateway's own docs.

| Gateway | Language | p99 (reported) | Max RPS (reported) | Memory (idle) | License |
|---------|----------|---------------|--------------------|---------------|---------|
| **Apiloom CE** | Go | < 1ms (proxy layer) | 738K target | ~30MB | Apache 2.0 |
| **KrakenD 2.x** | Go | ~2ms | ~70K | ~50MB | AGPL / Enterprise |
| **Kong 3.x** | Lua/OpenResty | ~10ms | ~30K | ~200MB | Apache 2.0 |
| **Apache APISIX 3.x** | Lua/OpenResty | ~5ms | ~45K | ~80MB | Apache 2.0 |

Sources and methodology notes are in `RESULTS.md §5`. Apiloom's proxy-layer numbers are not
directly comparable to gateway-level throughput figures from other tools.

## Hardware Disclosure

When sharing benchmark results, always report:

1. CPU model and core count
2. RAM
3. OS and kernel version
4. Go version
5. Whether running in Docker or bare metal
6. Git commit hash

Results without hardware context are not comparable. The `results/` directory should contain one file per release version with full environment details.

## Profiling

For deeper analysis beyond benchmarks:

```bash
# CPU profile
go test -bench=BenchmarkProxyHandler -cpuprofile=cpu.prof ./internal/gateway/...
go tool pprof cpu.prof

# Memory profile
go test -bench=BenchmarkProxyHandler -memprofile=mem.prof ./internal/gateway/...
go tool pprof mem.prof

# Trace
go test -bench=BenchmarkProxyHandler -trace=trace.out ./internal/gateway/...
go tool trace trace.out
```
