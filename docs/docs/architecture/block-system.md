---
sidebar_position: 2
title: Block System
description: The Block interface, block chain execution model, and built-in blocks.
---

# Block System

Every feature beyond core routing in FlexGateway is implemented as a Block. Blocks are composable middleware units that can inspect, modify, or reject requests before they reach the upstream backend.

## The Block Interface

```go
type Block interface {
    Name() string
    Middleware() proxy.Middleware
}
```

Two methods. That's the entire interface.

- **`Name()`** returns a unique string identifier (e.g., `"auth-jwt"`, `"rate-limit"`).
- **`Middleware()`** returns a Lura `proxy.Middleware` function that wraps the next proxy in the chain.

A Lura `proxy.Middleware` has this signature:

```go
type Middleware func(next ...proxy.Proxy) proxy.Proxy
```

Each middleware receives the next proxy in the chain and returns a new proxy that wraps it. This is standard function composition -- your block runs its logic, then calls the next proxy to continue the chain.

## Block Chain Execution

When a request matches a route, the gateway builds a chain of blocks based on the route's configuration:

```
Request -> [Auth] -> [RateLimit] -> [CORS] -> [SizeLimit] -> [Proxy] -> Backend
```

Blocks execute in configured order. Each block can:

1. **Pass through** -- call the next proxy unchanged.
2. **Modify** -- alter request headers, body, or metadata before calling the next proxy.
3. **Reject** -- return a `ProblemError` (RFC 7807) without calling the next proxy.
4. **Short-circuit** -- return a cached or synthetic response.

If a block rejects the request (e.g., auth failure, rate limit exceeded), the remaining blocks and the backend are never called.

## Built-in Blocks

### Authentication

| Block | Purpose |
|-------|---------|
| **Auth (JWT)** | Validates JWT tokens using JWKS endpoints. Supports RS256/ES256 with key rotation via background JWKS refresh. Filters algorithms to prevent `alg:none` attacks. |
| **Auth (API Key)** | Authenticates requests via `X-API-Key` header. Keys are bcrypt-hashed with an 8-character prefix cache for fast lookup. Supports key rotation with a configurable grace period. |

### Traffic Control

| Block | Purpose |
|-------|---------|
| **Rate Limit** | Dual-layer rate limiting: local token bucket for burst control plus distributed sliding window (backed by PostgreSQL) for aggregate enforcement. Supports per-consumer and per-key overrides via tier lookup. |
| **Circuit Breaker** | Wraps backend calls with [gobreaker](https://github.com/sony/gobreaker). Counts 5xx responses as failures. Opens circuit when failure threshold is exceeded, returning 503 until the backend recovers. |
| **Retry** | Retries failed backend calls with exponential backoff and jitter (crypto/rand). Respects a retry budget to prevent retry storms. Non-idempotent methods (POST/PATCH) are not retried by default. |
| **Timeout** | Wraps the proxy call in `context.WithTimeout`. Returns 504 if the deadline is exceeded. Supports per-route timeout overrides. |

### Security

| Block | Purpose |
|-------|---------|
| **CORS** | Cross-origin resource sharing. Handles preflight OPTIONS requests and sets `Access-Control-*` headers. Wildcard origins require explicit `AllowAllOrigins: true`. |
| **SizeLimit** | Limits request body size using `io.LimitReader(maxBytes+1)` to detect oversized bodies during streaming without buffering the entire body. |
| **mTLS** | Mutual TLS for upstream connections. Loads client certificates from disk per-connection to support certificate rotation without restart. |

### AI

| Block | Purpose |
|-------|---------|
| **AI Proxy** | LLM proxy supporting OpenAI, Anthropic, and Google Gemini. Handles provider-specific request/response translation, SSE streaming, token counting, and budget enforcement. See [AI Gateway Guide](../guides/ai-gateway.md). |

## How Blocks Compose with Lura

Lura's proxy middleware pattern is a chain of function wrappers. Each block's `Middleware()` returns a function that takes the next proxy(s) and returns a new proxy. The gateway composes these at route-build time:

```go
// Simplified block chain construction
var chain proxy.Proxy = backendProxy
for i := len(blocks) - 1; i >= 0; i-- {
    mw := blocks[i].Middleware()
    chain = mw(chain)
}
```

The result is a single proxy function that, when called, executes each block in order and eventually calls the backend.

## Writing Custom Blocks

To build your own block:

1. Implement the `Block` interface from `pkg/flex`.
2. Write tests using `flex.TestContext` (no gateway infrastructure needed).
3. Register the block in the gateway configuration.

For a step-by-step walkthrough, see the [Plugin Tutorial](../plugin-system/plugin-tutorial.md).
