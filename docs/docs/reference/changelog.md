---
sidebar_position: 10
title: Changelog
description: Release history for FlexGateway.
---

# Changelog

All notable changes to FlexGateway are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/). FlexGateway uses [Semantic Versioning](https://semver.org/).

## v0.5.0 -- 2026-03-22

The Ecosystem and Compliance release. Adds enterprise security, regulatory compliance, identity management, a plugin marketplace, and multi-cluster federation.

### Security Hardening

- **OWASP API Security Top 10 checks** -- Automated detection of broken authentication, injection, excessive data exposure, and other OWASP Top 10 risks. Implemented as a security block using Go regexp pattern matching.
- **DLP and PII masking** -- Field-level PII detection with configurable masking strategies (redact, hash, mask). Context-aware pattern matching.
- **Secret scanning** -- Entropy-based detection (Shannon entropy threshold 4.5) combined with pattern matching for AWS keys, tokens, and other secret formats in request/response bodies and headers.
- **Immutable audit log** -- Append-only audit trail with SHA-256 cryptographic hash chain. Each entry references the previous entry's hash, creating a tamper-evident record.
- **Security status endpoint** -- `/api/v1/security/status` returns current security posture.
- **Vulnerability disclosure program** -- Published SECURITY.md with reporting process, severity classification (CVSS v3.1), response timelines, and safe harbor provision.

### Compliance Framework

- **SOC 2 Type I controls** -- Automated evaluation of SOC 2 controls against live infrastructure via SystemProber interface. Decoupled from infrastructure for testability.
- **GDPR data residency routing** -- Route requests based on data residency requirements per region.
- **GDPR right to erasure** -- Ontology graph-powered cascade discovery using GetNeighbors. Audit entries anonymized (not deleted) during erasure.
- **Compliance dashboard API** -- MultiEvaluator merges SOC 2 and GDPR results into a unified report with readiness scores. In-memory TTL cache for evaluation results.
- **Envelope encryption** -- Secrets at rest encrypted with envelope encryption. LocalKEKProvider validates 32-byte KEK; KEKProvider interface ready for external KMS.

### Enterprise Identity

- **OIDC SSO** -- Single sign-on via OpenID Connect. JWKS caching with lestrrat-go/jwx/v2. Configuration validated by fetching OIDC discovery document before persisting.
- **SAML SSO** -- SAML 2.0 authentication using stdlib xml and crypto packages (CGO_ENABLED=0 compatible). Metadata validated on configuration.
- **SCIM 2.0 provisioning** -- User and group provisioning server. Constant-time token comparison for authentication. Store interfaces for testability.
- **IP allowlisting** -- Per-organization IP allowlists. Private range rejection at config time for SSRF prevention.
- **MFA enforcement** -- Per-organization MFA policy. Checks boolean mfa_verified claim; string values rejected for type safety.

### Partner Portal and Marketplace

- **Partner registration** -- Onboarding flow with status transitions (pending, approved, suspended) enforced at the domain level.
- **API product listings** -- Partners publish APIs with metadata, pricing, and documentation.
- **Plugin marketplace** -- Browse, install, rate, and purchase plugins. gRPC-only execution for security sandboxing.
- **Plugin sandbox** -- SandboxExecutor injection for plugin isolation. Plugin status transitions mirror listing lifecycle.
- **Revenue sharing** -- Configurable revenue split (default 85/15, 0% on first $100K lifetime). Stripe Connect integration for partner payouts.

### Scale and Launch

- **Multi-cluster federation** -- In-memory ClusterManager with health monitoring. ConfigSyncer pushes configuration to all clusters in parallel via errgroup.
- **A2A protocol** -- Agent-to-agent communication with per-pair token bucket rate limiting.
- **Benchmark suite** -- Load testing harness targeting 100K+ RPS with published results.
- **CNCF sandbox application** -- Prepared application document covering all required sections.

---

## v0.4.0 -- 2026-02-15

The Intelligence and Agents release. Adds ontology engine, AI-driven insights, agent governance, anomaly detection, and usage analytics.

### Added
- Ontology engine with graph-based API relationship modeling
- Automatic dependency discovery and impact analysis
- MCP server for AI agent tool access control
- Agent governance with policy enforcement
- Anomaly detection for traffic patterns
- Usage analytics and insights dashboard API

---

## v0.3.0 -- 2026-01-20

The Client Portal and Billing release. Adds developer portal, billing integration, self-service onboarding, and GitOps workflow.

### Added
- Client portal with self-service API key management
- Stripe billing integration with usage-based metering
- Self-service developer onboarding
- GitOps configuration workflow with CRD-based config
- Terraform provider for infrastructure-as-code

---

## v0.2.0 -- 2025-12-15

The Unified Control Plane release. Adds multi-tenancy, dashboard, MCP server, and client SDKs.

### Added
- Multi-tenant organization support with isolated namespaces
- Design system and component library
- Internal management portal (Next.js dashboard)
- MCP server for programmatic gateway management
- Client SDKs and developer experience improvements

---

## v0.1.0 -- 2025-11-01

Initial release. Core gateway functionality.

### Added
- Lura-based HTTP gateway with block chain request processing
- JWT and API key authentication blocks
- Rate limiting (local token bucket + distributed sliding window)
- Circuit breaker and retry with exponential backoff
- CORS, size limit, timeout, and mTLS blocks
- AI proxy block (OpenAI, Anthropic, Gemini) with multi-provider routing
- Token counting and budget enforcement
- SSE streaming with provider format translation
- PostgreSQL-backed configuration with LISTEN/NOTIFY hot reload
- Control plane REST API for routes, backends, consumers, AI models
- CLI tool (fgctl) for command-line management
- Kubernetes operator with Gateway API CRD support
- Plugin system (native Go + gRPC plugins)
- OpenTelemetry tracing and Prometheus metrics
- Helm charts and Docker images
- Docusaurus documentation site
