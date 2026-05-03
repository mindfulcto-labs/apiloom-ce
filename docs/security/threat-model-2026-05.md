# Apiloom CE Threat Model — 2026-05

- **Version:** 1.0 (initial)
- **Date:** 2026-05-03
- **Author:** CISO
- **Reviewer:** DevSecOps Engineer (pending sign-off)
- **Scope:** apiloom-ce monorepo at HEAD on `main` (commit `eb69909`)
  - Data plane: `cmd/gateway`, `internal/gateway`, `internal/block/*`, `internal/ai/*`
  - Control plane: `cmd/controlplane`, `internal/controlplane`, `internal/config`, `internal/plugin`
  - CLI: `cmd/fgctl`
  - Out of scope (this revision): `apiloom-ent` modules, dashboard, marketing site, customer-managed Kubernetes

This document is the W1 deliverable for VEN-3. It is a living document; revisions are tracked in git.

---

## 1. System overview

Apiloom CE is a Go-based, PostgreSQL-backed multi-tenant API gateway. Three binaries:

| Component | Listen | Purpose | Trust boundary |
|---|---|---|---|
| `apiloom-gateway` (data plane) | `:8080` | Proxies tenant traffic, runs the block chain (auth → rate-limit → CORS → size-limit → transforms → upstream proxy → AI proxy). | Internet-facing. All requests are untrusted. |
| `apiloom-controlplane` (control plane) | `:8081` | REST API for managing routes, backends, consumers, API keys, plugins, environments, AI models, token budgets, config versions. | **Currently treated as internal-only — see §6 finding CP-001.** |
| `fgctl` (CLI) | n/a | HTTP client for the control plane. Identifies the target org via the `--org` flag (passed as `X-Organization-ID`). | Operator workstation; trust derived from operator. |

PostgreSQL serves three roles: configuration store, LISTEN/NOTIFY hot-reload bus, and (optional) distributed rate-limit counter. Row-level security keys on `app.current_org` set per-transaction via `SET LOCAL`.

### 1.1 Data flow (data plane request)

```
Internet client
   │
   ▼
[ TLS terminator (out of scope; assumed in front) ]
   │
   ▼
apiloom-gateway :8080
   │ ──► observability (logging → tracing → metrics) middleware
   │
   ▼
DynamicHandler  ──► block chain
   │                 1. Auth (apikey OR jwt)
   │                 2. Rate limit (local + optional distributed)
   │                 3. CORS
   │                 4. Size limit
   │                 5. Transform / circuit-breaker / retry / timeout / mTLS
   │                 6. AI proxy (when route is /v1/chat/completions)
   ▼
Upstream backend (HTTP, gRPC, WebSocket, SSE) OR LLM provider
```

### 1.2 Data flow (control plane mutation)

```
operator (fgctl or HTTP client)
   │  X-Organization-ID: <attacker-controlled UUID>
   ▼
apiloom-controlplane :8081
   │  middleware: RequestID → RealIP → Logger → Recoverer → Timeout(30s)
   │  ────────────────────  NO AUTHN MIDDLEWARE (CP-001)  ────────────────────
   ▼
OrgContextMiddleware ──► sets app.current_org for every transaction
   │
   ▼
chi route handlers (routes / backends / consumers / keys / plugins / ai / config / env)
   │
   ▼
PostgreSQL (RLS scoped to app.current_org)
   │
   ▼
LISTEN/NOTIFY ──► gateway watcher ──► hot reload of DynamicHandler
```

---

## 2. Trust boundaries

| # | Boundary | Notes |
|---|---|---|
| TB-1 | Internet ↔ data plane :8080 | Hostile. All inputs validated. |
| TB-2 | Data plane ↔ upstream backends / LLM providers | Backends are tenant-configured; SSRF risk (see AI-001). |
| TB-3 | Data plane ↔ Postgres | Internal-tier; assumed mutual TLS or VPC. Postgres is privileged — it controls config. |
| TB-4 | Operator ↔ control plane :8081 | Currently has **no enforced auth**; a compensating network policy is the de-facto control (CP-001). |
| TB-5 | Tenant A ↔ Tenant B (inside data plane process) | Enforced by route config + `auth.GetConsumer(ctx)` + RLS at config-store boundary. |
| TB-6 | Plugin code ↔ host process | Native plugins run in-process with full process privileges (PL-001). |

---

## 3. Assets

| Asset | Confidentiality | Integrity | Availability |
|---|---|---|---|
| Tenant API keys (bcrypt-hashed at rest, raw shown once on create) | Critical | Critical | High |
| Tenant configuration (routes, backends, consumers, plugins) | High | Critical | High |
| Upstream LLM provider API keys (`ProviderConfig.APIKey`) | Critical | Critical | High |
| JWT signing keys (JWKS — fetched, not held) | n/a (public) | Critical | High |
| Audit log of agent / control-plane actions | High | Critical (hash chain) | Medium |
| In-flight tenant request bodies / responses | High | High | High |
| Postgres credentials (`FLEXGW_DATABASE_URL`) | Critical | Critical | High |
| OTLP / Prometheus telemetry | Medium | Medium | Medium |

---

## 4. STRIDE analysis

Each row: threat → affected component → existing mitigation → residual risk → tracking ID.

### 4.1 Data plane (`apiloom-gateway`)

| ID | STRIDE | Threat | Affected | Existing mitigation | Residual | Severity |
|---|---|---|---|---|---|---|
| DP-001 | **S**poofing | Cache-key auth bypass on API keys: cache lookup uses 8-char prefix only; if cache hits, **bcrypt is not re-verified** against the presented raw key. Any key whose first 8 chars match a recently-cached prefix authenticates as the cached consumer. | `internal/block/auth/apikey/apikey.go:154-173` | Cache TTL default 60s. Prefix uniqueness depends on RNG; no DB-level uniqueness was confirmed. | Open | **Critical** |
| DP-002 | **S**poofing / **R**epudiation | JWT block creates a fresh `uuid.New()` for every request rather than a stable consumer ID derived from `sub`/`iss`. Audit trail cannot link consecutive calls from the same identity; rate-limit keying by `consumer.ID` becomes per-request not per-user. | `internal/block/auth/jwt/jwt.go:110-115` | None. | Open | **High** |
| DP-003 | **T**ampering | JWT `extractClaims` writes `fmt.Sprintf("%v", val)` of arbitrary token claim values into upstream request headers. A claim containing CR/LF or control characters could enable HTTP header injection / response splitting against the upstream. | `internal/block/auth/jwt/jwt.go:221-234` | `ClaimMap` filters claim **names**, not values. Lura's serializer behavior is not enforced. | Open | **High** |
| DP-004 | **S**poofing | `extractClientIP` blindly trusts the **first** `X-Forwarded-For` value with no trusted-proxy allow-list. Attackers spoof XFF to (a) bypass per-IP rate limits by rotating values, (b) frame a victim IP in logs, (c) collide a victim into a higher-traffic bucket. | `internal/block/ratelimit/block.go:243-248` | None. Falls back to literal `"unknown"` when XFF absent — funneling all unauth traffic into one bucket. | Open | **High** |
| DP-005 | **D**oS | `LocalLimiterStore` has no eviction. Rate-limit keys include `req.Path`; a key built from `consumer.ID + path` has unbounded cardinality (path-fuzzing inflates the map indefinitely). 256 shards × unbounded ⇒ memory exhaustion. | `internal/block/ratelimit/limiter.go` | Sharding for contention only. | Open | **High** |
| DP-006 | **D**oS | When XFF absent the per-IP key is `ip:unknown:...`. All anonymous traffic shares a single bucket → tiny effective limit for legitimate anon users, or trivial whole-bucket exhaustion. | `internal/block/ratelimit/block.go:243-248` | None. | Open | Medium |
| DP-007 | **D**oS | Size-limit fast path checks `Content-Length`; slow path wraps body with `LimitReader(body, MaxBytes+1)`. **It does not return an error when the limit is hit on streaming**; instead the body is silently truncated. Downstream blocks that JSON-parse the truncated body may behave incorrectly; large chunked uploads cannot be rejected mid-stream. | `internal/block/sizelimit/sizelimit.go:64-68` | Content-Length fast path catches honest clients only. | Open | Medium |
| DP-008 | **I**nformation disclosure | API key may be passed via `?api_key=` query string. `stripAPIKeyParam` removes it before forwarding upstream, but the *gateway's own* observability middleware (logging / OTel span attributes) may already have captured the URL with the key. | `internal/block/auth/apikey/apikey.go:191-201, 286-293` | `redactKey` is used for explicit log lines; not for raw request URLs. | Open | Medium |
| DP-009 | **R**epudiation | JWT replay window is the token lifetime; there is no `jti` deny-cache. A stolen token is reusable until expiry. | `internal/block/auth/jwt/` | Standard JWT validation only. | Open | Medium |
| DP-010 | **E**levation of privilege | `extractKey` reads case-insensitively via `req.Headers[b.config.HeaderName]` — Go's http.Header normalizes, but this is map access not `Header.Get`. If a hostile client supplies both `X-API-Key` and `x-api-key`, behavior depends on Lura's header handling (which may or may not canonicalize). Worth a unit test. | `internal/block/auth/apikey/apikey.go:191-194` | None confirmed. | Open | Low |
| DP-011 | **D**oS / **T**ampering | OTel sampler is configurable (`FLEXGW_OTEL_SAMPLE_RATE` defaults to **1.0** = sample every span). Attackers can amplify storage / collector cost, and collected spans expose URLs / headers. | `cmd/gateway/main.go:293, internal/observability/` | Operator-configurable. | Open | Low |
| AI-001 | **I**nformation disclosure (SSRF) | `ValidateProviderURL` resolves the hostname *at config time* and rejects RFC-1918 / 169.254 / loopback. **DNS rebinding is unmitigated**: connection time uses default Go dialer that resolves again. Also misses Alibaba metadata (`100.100.100.200`), `0.0.0.0`, `0`, IPv6 `[::]`. If hostname doesn't resolve at validation, the URL is allowed. | `internal/ai/provider/provider.go:64-115` | Static ranges + name match for `localhost` and `metadata.google.internal`. | Open | **High** |
| AI-002 | **I**nformation disclosure | Upstream LLM API keys (`ProviderConfig.APIKey`) live in DB and process memory. JSON tag is `json:"-"` (good), but no envelope encryption at rest; whoever has DB read has the keys. | `internal/ai/provider/provider.go:30-36` | DB-tier access control + RLS. | Open | Medium |
| PL-001 | **E**levation of privilege | Native plugins are loaded in-process via `BlockFactory`. A compromised plugin has full process privileges (DB pool, filesystem, network). gRPC plugin host is a placeholder. WASM is present (`internal/plugin/wasm`) but not verified isolated in this audit. | `internal/plugin/registry.go` | Operator must vet plugins. | Open | Medium |

### 4.2 Control plane (`apiloom-controlplane`)

| ID | STRIDE | Threat | Affected | Existing mitigation | Residual | Severity |
|---|---|---|---|---|---|---|
| CP-001 | **S**poofing / **E**oP | **No authentication on the REST API.** `Server.Handler()` mounts only `RequestID, RealIP, Logger, Recoverer, Timeout, OrgContextMiddleware`. Any caller with network access can supply any UUID in `X-Organization-ID` and CRUD all routes, backends, consumers, API keys, plugins, AI models, token budgets, and config history of that org. | `internal/controlplane/server.go:48-108` | Network policy / VPC peering is the only line of defense. `fgctl` sets the header from a flag with no token. | Open | **Critical** |
| CP-002 | **T**ampering / **I**nformation disclosure | `X-Organization-ID` is fully attacker-controlled. Cross-tenant access is *only* gated by knowledge of a tenant's UUID. UUIDs are non-secret (e.g., they appear in API responses, logs). RLS on Postgres is downstream of the header — once the header is set, RLS happily scopes to the chosen tenant. | `internal/controlplane/server.go:155-174` | UUID v4 entropy. | Open | **Critical** (combined with CP-001) |
| CP-003 | **D**oS | Control plane has no rate limiting and no body-size limit middleware. A flood of `POST /api/v1/plugins` or large JSON bodies can OOM or saturate Postgres. | `internal/controlplane/server.go` | `Timeout(30s)` only. | Open | High |
| CP-004 | **I**nformation disclosure | `chi` `middleware.Logger` logs request lines; if a future endpoint accepts an API key in path or query, it would land in logs. (Not currently the case but worth a guardrail.) | `internal/controlplane/server.go:54` | None. | Open | Low |
| CP-005 | **R**epudiation | Audit-log endpoint / hash-chain validation is an OKR target (W1) but is not visible in this audit. Without a SOC 2-grade audit log, control-plane mutations cannot be attributed. | n/a | None. | Open | High |
| CP-006 | **T**ampering | `SetOrgContext` uses `fmt.Sprintf("SET LOCAL app.current_org = '%s'", uuid)`. The UUID is `uuid.UUID.String()` (typed return → injection-safe). **However**, if any future contributor passes a string that is not a typed UUID, this becomes a SQLi pivot. Add a static check. | `internal/config/org_context.go:35-43` | Comment + type signature. | Open | Low |
| CP-007 | **I**nformation disclosure | `handleHealthz` returns 503 with `Detail: "connection failed"` when Postgres is down — fine. But `middleware.Recoverer` from chi prints stack traces to the response in dev modes if misconfigured. Verify production wiring. | `internal/controlplane/server.go:54-141` | Default chi behavior. | Open | Low |
| CP-008 | **D**oS | `pgxpool` MaxConns = 10 (control plane). With no rate limit, a slow-loris on long-running endpoints (`POST /consumers` with metadata processing) saturates the pool, blocking all tenants. | `cmd/controlplane/main.go:76-79` | Pool ceiling itself. | Open | Medium |

### 4.3 fgctl

| ID | STRIDE | Threat | Affected | Existing mitigation | Residual | Severity |
|---|---|---|---|---|---|---|
| CL-001 | **S**poofing | The CLI sends only `X-Organization-ID`. There is no auth token, no mTLS, no SPIFFE id, no SSH-tunnel requirement. A leaked org UUID + control-plane network access ⇒ full CRUD. | `cmd/fgctl/client.go` | Inherits CP-001 mitigation. | Open | **Critical** (same root cause as CP-001) |
| CL-002 | **I**nformation disclosure | Any future CLI subcommand that prints `APIKeyCreateResponse.Key` to stdout will leak the raw key into shell history / CI logs. Worth a UX guardrail (`--output-file`, redact-by-default unless `--show-key`). | `internal/controlplane/handler_consumers.go:211-214` | None. | Open | Medium |

### 4.4 Cross-cutting / company-tier

| ID | STRIDE | Threat | Existing mitigation | Severity |
|---|---|---|---|---|
| XC-001 | **R**epudiation | `SECURITY.md` advertises **`security@flexgateway.io`** for vulnerability reports, contradicting the OKR target of `security@apiloom.io`. Reporters will email a domain we do not control or that is dead. | None | **High** (compliance/reputational) |
| XC-002 | **I**nformation disclosure | Any plugin / block calling `slog.*` with raw request bodies risks leaking PII or secrets. Audit found no current offenders, but no enforced log-allowlist exists. | `redactKey` exists but is API-key specific. | Medium |
| XC-003 | **R**epudiation | No documented incident-response runbook in repo (`docs/security/` was empty before this commit). | None | Medium |

---

## 5. OWASP API Security Top 10 (2023) mapping

| OWASP item | Status | Evidence / gaps |
|---|---|---|
| **API1: Broken Object Level Authorization** | **Partial** | `verifyKeyOwnership` (handler_consumers.go:317) explicitly checks `key.ConsumerID == consumerID` and returns 404 to avoid leaking existence. Good pattern. RLS on Postgres scopes by org. **But CP-001 nullifies BOLA at the org level — attacker chooses the org.** |
| **API2: Broken Authentication** | **Fail** | DP-001 (cache bypass), DP-002 (JWT consumer ID drift), CP-001 (no control-plane auth), CL-001 (no CLI auth). |
| **API3: Broken Object Property Level Authorization** | Not assessed in this revision | Need to walk JSON bodies of consumer/route updates for mass-assignment risk; deferred to W2. |
| **API4: Unrestricted Resource Consumption** | **Fail** | DP-005 (limiter map unbounded), DP-007 (size-limit truncation not error), CP-003 (no CP rate limit), CP-008 (pool DoS). |
| **API5: Broken Function Level Authorization** | **Fail** | All control-plane functions are publicly callable (CP-001). No role separation. |
| **API6: Unrestricted Access to Sensitive Business Flows** | Partial | Rate-limit pipeline in place but bypassable via XFF spoofing (DP-004). |
| **API7: Server-Side Request Forgery** | **Partial** | `ValidateProviderURL` blocks RFC-1918 + GCP metadata. Misses Alibaba metadata, `0.0.0.0`, `[::]`, DNS rebinding (AI-001). |
| **API8: Security Misconfiguration** | Partial | OTel sample rate defaults to 1.0; CORS defaults restrictive (good); chi `middleware.Recoverer` behavior in production not verified (CP-007). |
| **API9: Improper Inventory Management** | Partial | OpenAPI spec lives in `api/openapi`; no documented public-API surface vs internal vs deprecated. |
| **API10: Unsafe Consumption of APIs** | Partial | Upstream LLM responses are translated and surfaced to clients; AI-002 covers stored API keys. No documented allowlist of upstream response sizes / content types. |

### 5.1 OWASP Web Top 10 (2021) cross-reference (data plane is web-facing)

| Item | Status | Notes |
|---|---|---|
| A01 Broken Access Control | Fail | CP-001, CP-002, DP-001 |
| A02 Cryptographic Failures | Pass with caveats | bcrypt for API keys ✓; JWKS ✓; LLM keys not encrypted at rest (AI-002) |
| A03 Injection | Pass | Parametrized SQL via pgx; the one `Sprintf` use in `org_context.go` is type-safe (CP-006). Header injection via JWT claims is open (DP-003). |
| A04 Insecure Design | Fail | Control plane shipped without auth (CP-001) |
| A05 Security Misconfiguration | Partial | OTel default 1.0; recoverer behavior unverified |
| A06 Vulnerable Components | Not assessed | SBOM + dependency scan deliverable VEN-? (separate); add to W2 |
| A07 Identification & Auth Failures | Fail | DP-001, DP-002, DP-009 |
| A08 Software & Data Integrity Failures | Partial | No verified plugin signing (PL-001) |
| A09 Security Logging & Monitoring Failures | Partial | OTel + slog present; no audit-log hash chain wired in CE (CP-005) |
| A10 SSRF | Partial | AI-001 |

---

## 6. Prioritized fix list

Severity legend: **P0 = block any external launch / customer pilot.** P1 = before SOC 2 Type 1 evidence collection completes. P2 = before GA pricing / public marketing. P3 = hygiene.

### P0 — Must fix before any customer-reachable deployment

1. **CP-001 / CL-001 — Add authentication to the control plane.**
   - Owner: DevSecOps + Chief Architect.
   - Direction: bearer token (operator API keys, hashed at rest like consumer keys), or mTLS via SPIFFE. Token must scope to organization(s); `X-Organization-ID` becomes claim-derived, not header-asserted.
   - Acceptance: every `/api/v1/*` route returns 401 without a valid token; org claim must match the resource's `organization_id`.
2. **DP-001 — Fix API-key cache to re-verify the raw key.**
   - Owner: data-plane team.
   - Direction: cache value should hold the matched bcrypt hash (or a fast secondary HMAC of the raw key keyed by a server-side secret). On hit, recompute and constant-time compare.
   - Acceptance: a unit test where two keys share an 8-char prefix (or an attacker submits `<prefix><random>`) must fail authentication on cache hit.
3. **DP-003 — Reject CR/LF in JWT-derived header values.**
   - Direction: validate `strings.IndexAny(strVal, "\r\n\x00") == -1` before injection; drop the claim and log a warning.
   - Acceptance: fuzz test passes; manual repro using `email: "attacker@x\r\nX-Admin: true"` cannot inject.
4. **AI-001 — Harden SSRF.**
   - Direction: custom `net.Dialer.Control` that re-validates the resolved IP at `Connect`; add Alibaba metadata, `0.0.0.0`, IPv6 `[::]`, AWS `169.254.170.2` (already covered by /16 ✓), Oracle `192.0.0.192`. Reject hostnames that do not resolve at validation time **unless** runtime dialer enforces.
   - Acceptance: DNS-rebinding integration test (TTL-0 record flipping public→127.0.0.1) is rejected at connect.
5. **XC-001 — Update SECURITY.md to `security@apiloom.io`.**
   - One-line fix; required for the CNCF Sandbox security disclosure deliverable.

### P1 — Before SOC 2 Type 1 evidence collection completes

6. **CP-003 / CP-008 — Add rate limiting + body-size limit to the control plane.** Reuse the data-plane sizelimit + a simple chi-rate-limit middleware. Set MaxBytes ≤ 1 MiB.
7. **DP-002 — Derive consumer ID deterministically from `iss+sub`.** Persist the mapping; all downstream rate-limit + audit log keying must be stable per identity.
8. **DP-004 / DP-006 — Configurable trusted-proxies list for XFF.** Use leftmost-non-trusted algorithm; reject `unknown` as a key (key on raw socket peer when XFF unparseable).
9. **DP-005 — Bound the local rate-limit map.** Use an LRU with a cap (e.g. 1M entries) and a periodic sweeper; emit a `ratelimit_evictions_total` metric.
10. **DP-007 — Make size-limit fail loudly on streaming overflow.** When the LimitReader returns the +1 byte, return 413 ProblemError; do not let truncated bodies reach downstream parsers.
11. **CP-005 — Wire the SHA-256 audit-log hash chain (OKR deliverable).** Cover all control-plane mutations; expose a verification endpoint and write end-to-end tests.
12. **PL-001 — Plugin signing + allow-list.** Require a configured signature for native plugins; disable arbitrary plugin install via API by default.

### P2 — Before public GA / Aegis design partner traffic

13. **AI-002 — Envelope-encrypt upstream LLM API keys at rest** using a KEK held outside Postgres (cloud KMS).
14. **DP-008 — Strip `api_key` query param before observability middleware sees the URL.** Or disable `AllowQueryParam` by default.
15. **DP-009 — JWT `jti` deny-cache** with a TTL ≥ token lifetime. Pluggable backend (in-mem for CE, Redis for ent).
16. **DP-011 — Lower default OTel sample rate** to 0.1 with an env override; document.
17. **CL-002 — Redact `APIKeyCreateResponse.Key` by default in `fgctl`** and write to a 0600 file unless `--show-key`.
18. **CP-007 — Pin `chi/middleware.Recoverer` to a non-debug PrintStack=false in production builds**; add a regression test.
19. **CP-006 — Add a static check / linter rule** that `SET LOCAL app.current_org = ...` callers pass only `uuid.UUID.String()`.

### P3 — Hygiene

20. **DP-010** — Unit test for case-collision in `extractKey` (`X-API-Key` vs `x-api-key`).
21. **XC-002** — Document a logging allow-list in `docs/security/logging.md`; add a CI grep that fails on `slog.*` calls touching `req.Body`, `Authorization`, `password`.
22. **XC-003** — Author `docs/security/incident-response.md` with severity ladder, comms tree, and customer-notification template.
23. **API3 (Broken object-property auth)** — Audit `UpdateConsumerRequest`, `UpdateRouteRequest`, etc. for mass-assignment risk in W2.
24. **API9 (Inventory)** — Tag every `internal/controlplane/handler_*` with `Stability: stable|beta|internal`; surface in the OpenAPI doc.

---

## 7. Out-of-scope items recorded for later threat modeling

- `apiloom-ent` Aegis modules (`internal/governance`, `internal/guardrail`, `internal/ontology`) — separate threat model owed before Aegis design-partner kickoff.
- Dashboard (Next.js) — OWASP Top 10 sweep tracked separately under VEN-?.
- Marketing site `apiloom.io` — content-risk only; no app surface.
- Helm charts / K8s manifests — pen-test scope (separate engagement).
- Customer-managed deploys — covered by deployment hardening guide (W3 deliverable).

---

## 8. Sign-off

- [ ] CISO (author) — 2026-05-03
- [ ] DevSecOps Engineer — pending
- [ ] CTO — informational

Once DevSecOps signs off, the prioritized fix list converts to issues (one per finding ID) under the `security` label, owned by the relevant team. P0s block the first paying enterprise customer engagement.
