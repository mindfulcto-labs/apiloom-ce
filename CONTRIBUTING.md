# Contributing to FlexGateway

Thank you for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- **Go 1.25+** (check with `go version`)
- **Docker** (for running PostgreSQL and building containers)
- **PostgreSQL 14+** (or use Docker Compose -- recommended)
- **golangci-lint** (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

## Getting Started

### Fork and Clone

```bash
# Fork the repository on GitHub, then:
git clone https://github.com/<your-username>/flex-gateway.git
cd flex-gateway
```

### Development Environment Setup

Start PostgreSQL with Docker Compose:

```bash
docker compose up -d
```

Set the database connection string:

```bash
export FLEXGW_DATABASE_URL="postgres://flexgw:flexgw_dev@localhost:5432/flexgateway?sslmode=disable"
```

Apply database migrations:

```bash
make migrate-up
```

### Build and Test

```bash
make build              # Build all binaries (gateway, control plane, fgctl)
make test               # Run unit tests with race detector
make lint               # Run golangci-lint and go vet
make test-integration   # Run integration tests (requires Postgres)
make test-short         # Run tests without Postgres dependency
make bench              # Run benchmarks
make security           # Run govulncheck and gosec
make all                # lint + test + build
```

Verify your environment works:

```bash
make all
```

## Development Workflow

### Test-Driven Development

TDD is mandatory for all contributions. The workflow:

1. Write a failing test that describes the expected behavior.
2. Write the minimal code to make the test pass.
3. Refactor while keeping all tests green.

No pull request will be merged without corresponding tests.

### Code Standards

These are enforced by CI and code review:

- **Error wrapping:** `fmt.Errorf("doing X: %w", err)`. Sentinel errors go in `internal/domain/errors.go`.
- **Context:** First parameter in every function, propagated through all layers.
- **Context values:** Unexported typed keys (`type ctxKey struct{}`), with accessor functions.
- **Interfaces:** Defined in the consumer package, not the provider. Maximum 1-3 methods; use composition for optional capabilities.
- **Imports:** Standard library, blank line, external packages, blank line, internal packages.
- **Functions:** Under 50 lines. Extract helpers when a function grows beyond that.
- **No CGO:** All production builds use `CGO_ENABLED=0`.
- **No bare goroutines:** Use `errgroup.Group` or `sync.WaitGroup` with context.
- **No `math/rand` for security:** Use `crypto/rand`.
- **No string interpolation in SQL:** Parameterized queries (`$1`, `$2`) only.

See [CLAUDE.md](CLAUDE.md) for the full set of rules and project structure.

### Branch Naming

- `feature/<description>` -- New functionality
- `fix/<description>` -- Bug fixes
- `docs/<description>` -- Documentation changes

### Commit Messages

Use conventional commits:

```
feat(auth): add JWT refresh token rotation
fix(ratelimit): correct sliding window boundary calculation
test(proxy): add benchmark for round-robin selection
docs(readme): update architecture diagram
refactor(config): extract NOTIFY listener into separate struct
```

## Pull Request Process

1. Create a branch from `main` with the appropriate prefix.
2. Make your changes following the code standards above.
3. Ensure all checks pass locally: `make all`
4. Push your branch and open a pull request.
5. Fill out the PR template -- describe what changed and why.
6. At least one maintainer must approve before merge.
7. CI must pass (lint, unit tests, integration tests, build).

### What Makes a Good PR

- Solves one problem. Split unrelated changes into separate PRs.
- Includes tests. Both positive and negative cases.
- Has a clear description of the problem and solution.
- Follows existing patterns in the codebase.

## What to Contribute

### Good First Issues

Look for issues labeled `good first issue`. These are scoped, well-defined tasks suitable for newcomers.

### Feature Requests

Open a feature request issue using the template. Describe the problem you are trying to solve, not just the solution you want.

### Plugin Ideas

FlexGateway's plugin system supports native Go plugins and gRPC plugins. If you have an idea for a plugin, open a plugin request issue. The plugin SDK is in `pkg/flex/`.

### Documentation

Documentation improvements are always welcome. Fix typos, clarify explanations, add examples.

## Code Review Expectations

- All PRs are reviewed by at least one maintainer.
- Reviewers check for correctness, test coverage, adherence to code standards, and potential security issues.
- Address review feedback by pushing new commits (do not force-push during review).
- Reviewers may request changes. This is normal and not a judgment on your work.

## Project Structure

```
cmd/gateway/           -- Data plane binary
cmd/controlplane/      -- Control plane binary
cmd/fgctl/             -- CLI binary
internal/block/        -- Block implementations (auth, ratelimit, aiproxy, cors...)
internal/config/       -- Postgres config store, LISTEN/NOTIFY, hot reload
internal/ai/           -- LLM provider adapters, token counting, budget enforcement
internal/plugin/       -- Plugin SDK, gRPC plugin host, lifecycle management
internal/domain/       -- Domain types, sentinel errors
internal/observability/ -- OTel setup, metrics, logging
pkg/flex/              -- Public plugin SDK (the only exported package)
```

## Getting Help

- Open a GitHub Discussion for questions.
- Check existing issues before opening a new one.
- Read the [Architecture documentation](docs/ARCHITECTURE.md) for design context.

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
