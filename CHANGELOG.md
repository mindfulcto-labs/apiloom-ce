# Changelog

All notable changes to Apiloom Gateway CE are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each release is tagged in git as `vX.Y.Z` and published as a [GitHub Release](https://github.com/cto-venkat/apiloom-ce/releases) with signed container images and SPDX SBOMs (see [docs/release-process.md](docs/release-process.md)).

## [Unreleased]

Tracking work in flight. See [ROADMAP.md](ROADMAP.md) for the current quarter's themes.

## [0.7.0] -- 2026-03-23

First public release of Apiloom Gateway CE under the Apache 2.0 license. Consolidates the gateway, control plane, plugin SDK, CLI, Helm chart, and benchmark suite into one repository, drops the internal "FlexGateway" code name, and folds in the security and compliance surface developed across earlier internal releases.

### Added
- Public repository at `github.com/cto-venkat/apiloom-ce` with Apache 2.0 license, contributor docs, governance, security policy, and CNCF-aligned project files.
- GitHub Actions for CI (lint, test matrix on Postgres 15/16/17, build, web build, benchmark regression, staging deploy), release (multi-arch images, cosign signing, SPDX SBOM via syft, Helm chart push to GHCR), and security (govulncheck, gosec).
- Issue and pull request templates, Dependabot config, and a CODEOWNERS-style maintainer file.
- Benchmark harness reproducing 738K+ RPS auth+rate-limit and 1.2M+ RPS direct-proxy on M3 Pro hardware. See `benchmarks/`.

### Changed
- Renamed module path from internal `flexgateway` to public `apiloom-ce`. Existing import paths are not preserved.
- Marketing/enterprise pointers in the README route to `apiloom.io` rather than the internal enterprise repository.

### Known issues
- A handful of files still reference the legacy "FlexGateway" name and `flexgateway.io` contacts (`CONTRIBUTING.md`, `GOVERNANCE.md`, `SECURITY.md`, the release workflow's tarball names, the Makefile module path, the docs site). Tracked in [docs/cncf/gap-analysis-2026-05.md](docs/cncf/gap-analysis-2026-05.md) and being cleaned up across follow-up issues.

## [0.5.0] -- 2026-03-22

The Ecosystem and Compliance release.

### Added
- **Security hardening:** OWASP API Security Top 10 detection block; field-level DLP and PII masking; entropy-based secret scanning (Shannon threshold 4.5) plus pattern matching for AWS keys and tokens; append-only audit log with SHA-256 hash chain; `/api/v1/security/status` endpoint; published vulnerability disclosure policy with CVSS v3.1 severity tiers and safe harbor.
- **Compliance:** Automated SOC 2 Type I control evaluation against live infrastructure via the `SystemProber` interface; GDPR data-residency routing; GDPR right-to-erasure with cascade discovery via the ontology graph (audit entries anonymized rather than deleted); unified compliance dashboard API with TTL-cached results; envelope encryption with `LocalKEKProvider` (32-byte KEK validation) and a `KEKProvider` interface ready for external KMS.
- **Enterprise identity:** OIDC SSO with JWKS caching (`lestrrat-go/jwx/v2`) and pre-persist discovery validation; SAML 2.0 SSO using stdlib `xml`/`crypto` (no CGO); SCIM 2.0 user/group provisioning with constant-time token comparison; per-organization IP allowlists with private-range rejection (SSRF prevention); per-organization MFA enforcement with strict boolean `mfa_verified` claim handling.
- **Partner and marketplace:** Partner registration with state-machine status transitions; API product listings; plugin marketplace with gRPC-only execution sandboxing; configurable revenue split (default 85/15, 0% on first $100K lifetime) with Stripe Connect payouts.
- **Scale:** In-memory `ClusterManager` with health monitoring; parallel `ConfigSyncer` via `errgroup`; agent-to-agent (A2A) protocol with per-pair token bucket rate limiting.

## [0.4.0] -- 2026-02-15

The Intelligence and Agents release.

### Added
- Ontology engine with graph-based API relationship modeling and automatic dependency/impact analysis.
- MCP server for AI agent tool access control.
- Agent governance with policy enforcement.
- Anomaly detection on traffic patterns.
- Usage analytics and insights dashboard API.

## [0.3.0] -- 2026-01-20

The Client Portal and Billing release.

### Added
- Client portal with self-service API key management.
- Stripe billing integration with usage-based metering.
- Self-service developer onboarding.
- GitOps configuration workflow with CRD-based config.
- Terraform provider for infrastructure-as-code.

## [0.2.0] -- 2025-12-15

The Unified Control Plane release.

### Added
- Multi-tenant organization support with isolated namespaces.
- Design system and component library.
- Internal management portal (Next.js dashboard).
- MCP server for programmatic gateway management.
- Client SDKs and developer-experience improvements.

## [0.1.0] -- 2025-11-01

Initial release. Core gateway functionality.

### Added
- Lura-based HTTP gateway with block-chain request processing.
- JWT and API key authentication blocks.
- Rate limiting (local token bucket + distributed sliding window).
- Circuit breaker and retry with exponential backoff.
- CORS, size limit, timeout, and mTLS blocks.
- AI proxy block (OpenAI, Anthropic, Gemini) with multi-provider routing, token counting, and budget enforcement.
- SSE streaming with provider format translation.
- PostgreSQL-backed configuration with LISTEN/NOTIFY hot reload.
- Control plane REST API for routes, backends, consumers, and AI models.
- CLI (`fgctl`) for command-line management.
- Kubernetes operator with Gateway API CRD support.
- Plugin system (native Go and gRPC plugins).
- OpenTelemetry tracing and Prometheus metrics.
- Helm charts and Docker images.
- Docusaurus documentation site.

[Unreleased]: https://github.com/cto-venkat/apiloom-ce/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.7.0
[0.5.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.5.0
[0.4.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.4.0
[0.3.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.3.0
[0.2.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.2.0
[0.1.0]: https://github.com/cto-venkat/apiloom-ce/releases/tag/v0.1.0
