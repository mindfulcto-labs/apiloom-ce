MODULE   := github.com/flexgateway/flexgateway
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GOFLAGS  := -trimpath
TESTFLAGS := -race -count=1

.PHONY: all build test test-short test-integration lint bench generate docker docker-debug helm-lint security migrate-up migrate-down migrate-create clean

all: lint test build

## Build
build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/flexgateway ./cmd/gateway
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/flexcontrolplane ./cmd/controlplane
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/fgctl ./cmd/fgctl

## Test
test:
	go test $(TESTFLAGS) ./...

test-short:
	go test -short -count=1 ./...

test-integration:
	go test $(TESTFLAGS) -tags=integration ./...

## Lint
lint:
	golangci-lint run ./...
	go vet ./...

## Benchmark
bench:
	go test -bench=. -benchmem -count=5 ./... | tee bench.txt

bench-compare:
	@test -f bench-baseline.txt || (echo "No baseline. Run 'make bench' and 'cp bench.txt bench-baseline.txt' first."; exit 1)
	go test -bench=. -benchmem -count=5 ./... | tee bench-new.txt
	benchstat bench-baseline.txt bench-new.txt

## Code Generation
generate:
	go generate ./...

## Database Migrations (golang-migrate)
MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate-up:
	$(MIGRATE) -path internal/config/migrations -database "$$FLEXGW_DATABASE_URL" up

migrate-down:
	$(MIGRATE) -path internal/config/migrations -database "$$FLEXGW_DATABASE_URL" down 1

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=create_apis"; exit 1)
	$(MIGRATE) create -ext sql -dir internal/config/migrations -seq $(NAME)

## Docker
docker:
	docker build --target gateway -t flexgateway:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .
	docker build --target controlplane -t flexcontrolplane:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .

docker-debug:
	docker build -f Dockerfile.debug --target gateway -t flexgateway:$(VERSION)-debug .

## Helm
helm-lint:
	helm lint charts/flexgateway
	helm template flexgateway charts/flexgateway > /dev/null

## Security
security:
	govulncheck ./...
	golangci-lint run --enable gosec ./...

## dev: Start full local dev stack (compose up + seed data)
dev:
	docker compose up -d --build
	@echo "Waiting for controlplane to be ready..."
	@timeout 60 bash -c 'until curl -sf http://localhost:18081/healthz > /dev/null; do sleep 2; done'
	@echo "Seeding demo data..."
	cd web && pnpm exec tsx scripts/seed-demo-data.ts
	@echo ""
	@echo "  Portal:        http://localhost:13000"
	@echo "  Gateway:       http://localhost:18080"
	@echo "  Control Plane: http://localhost:18081"
	@echo "  OTel zPages:   http://localhost:55679"

## dev-down: Stop and remove all local dev containers
dev-down:
	docker compose down -v

## dev-logs: Tail logs from all services
dev-logs:
	docker compose logs -f --tail=50

## Clean
clean:
	rm -rf bin/ bench.txt bench-new.txt coverage.out
