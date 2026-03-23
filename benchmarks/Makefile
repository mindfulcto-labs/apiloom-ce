GATEWAY_URL ?= http://localhost:18080
K6_VUS      ?= 100
K6_DURATION ?= 30s

.PHONY: bench-go bench-k6 bench-all bench-report bench-full bench-quick bench-rps

## Go microbenchmarks (proxy handler, block chain, route matching, JSON marshaling)
bench-go:
	cd .. && go test -bench=. -benchmem -count=5 ./internal/gateway/... | tee benchmarks/results/go-bench.txt

## k6 load test against a running gateway
bench-k6:
	k6 run \
		--env GATEWAY_URL=$(GATEWAY_URL) \
		--summary-trend-stats="avg,min,med,max,p(90),p(95),p(99)" \
		k6/load-test.js

## k6 quick smoke test (lower load, shorter duration)
bench-k6-smoke:
	k6 run \
		--env GATEWAY_URL=$(GATEWAY_URL) \
		--vus $(K6_VUS) \
		--duration $(K6_DURATION) \
		k6/load-test.js

## Run both Go benchmarks and k6 load test
bench-all: bench-go bench-k6

## Format Go benchmark results into markdown
bench-report:
	@echo "# Go Benchmark Results"
	@echo ""
	@echo '```'
	@cat results/go-bench.txt 2>/dev/null || echo "No results yet. Run 'make bench-go' first."
	@echo '```'
	@echo ""
	@echo "Generated: $$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

## RPS benchmark suite (100K+ target) — full run with 5 iterations
bench-full:
	cd .. && go test -bench='Benchmark(ProxyPassthrough|AuthRateLimit|AIProxy|SecurityBlock)' \
		-benchmem -count=5 -timeout=10m ./benchmarks/... | tee benchmarks/results/rps-bench.txt

## RPS benchmark suite — quick single-count run for CI
bench-quick:
	cd .. && go test -bench='Benchmark(ProxyPassthrough|AuthRateLimit|AIProxy|SecurityBlock)' \
		-benchmem -count=1 -timeout=5m ./benchmarks/...

## RPS benchmarks only (alias for bench-full)
bench-rps: bench-full
