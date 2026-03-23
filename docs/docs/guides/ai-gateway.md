---
sidebar_position: 3
title: AI Gateway Guide
description: Multi-provider LLM routing, token budgets, SSE streaming, and fallback chains.
---

# AI Gateway Guide

FlexGateway includes a built-in AI proxy that routes requests to multiple LLM providers through a single OpenAI-compatible endpoint. Instead of building custom LLM proxy logic in your application, you configure models and budgets in the gateway and call one API.

## What It Does

The AI Gateway sits between your application and LLM providers. It handles:

- **Provider translation** -- send OpenAI-format requests; the gateway translates to Anthropic or Gemini wire formats automatically.
- **Model routing** -- distribute requests across providers using round-robin, cost-optimized, latency-optimized, or capability-based strategies.
- **Fallback chains** -- if a provider fails or is circuit-broken, the gateway tries the next provider in the chain.
- **Token counting** -- pre-estimates token usage before the request (using tiktoken cl100k_base) and reconciles with actual usage from the provider response.
- **Budget enforcement** -- per-consumer and per-team token and cost budgets with configurable hard/soft limits.
- **SSE streaming** -- streams responses from all providers using Server-Sent Events, translating provider-specific event formats inline.

## Supported Providers

| Provider | Models | Auth |
|----------|--------|------|
| **OpenAI** | GPT-4o, GPT-4, GPT-3.5-turbo, etc. | `OPENAI_API_KEY` header or env var |
| **Anthropic** | Claude 3.5 Sonnet, Claude 3 Opus, Haiku, etc. | `ANTHROPIC_API_KEY` header or env var |
| **Google Gemini** | Gemini 1.5 Pro, Gemini 1.5 Flash, etc. | API key or service account (configurable via `auth_mode`) |

## Quick Setup

### 1. Register Models

Register each model you want to route to:

```bash
# OpenAI GPT-4o
curl -X POST http://localhost:8081/api/v1/ai/models \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "model_name": "gpt-4o",
    "display_name": "GPT-4o",
    "input_price_per_token": 0.000005,
    "output_price_per_token": 0.000015,
    "capabilities": ["chat", "function_calling", "vision"],
    "weight": 1
  }'

# Anthropic Claude 3.5 Sonnet
curl -X POST http://localhost:8081/api/v1/ai/models \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "anthropic",
    "model_name": "claude-3-5-sonnet-20241022",
    "display_name": "Claude 3.5 Sonnet",
    "input_price_per_token": 0.000003,
    "output_price_per_token": 0.000015,
    "capabilities": ["chat", "function_calling", "vision"],
    "weight": 1
  }'
```

### 2. Set Provider API Keys

Pass provider credentials as environment variables to the gateway:

```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
GOOGLE_API_KEY=AIza...
```

### 3. Send Requests

The gateway exposes an OpenAI-compatible `/v1/chat/completions` endpoint:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-consumer-key" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Explain TCP congestion control in two sentences."}
    ]
  }'
```

To stream the response:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-consumer-key" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Write a haiku about distributed systems."}
    ],
    "stream": true
  }'
```

## Model Routing Strategies

When multiple models from different providers are registered, you can control how the gateway selects a provider:

| Strategy | Behavior |
|----------|----------|
| **Round-robin** | Distributes requests evenly across providers with matching models. Default strategy. |
| **Cost-optimized** | Routes to the cheapest provider for the requested model. Uses `input_price_per_token` and `output_price_per_token` from the model registry. |
| **Latency-optimized** | Routes to the provider with the lowest observed latency. Based on a running average of response times. |
| **Capability-based** | Routes to providers whose models have the required capabilities (e.g., `vision`, `function_calling`). Set `required_capabilities` on the request. |

Capability-based routing uses a `RequiredCapabilities` field on the request. The gateway filters models to those that match all required capabilities before applying the selected routing strategy.

## Fallback Chains

If a provider returns an error or its circuit breaker is open, the gateway tries the next available provider:

```
Request -> Provider A (circuit open) -> Provider B (500 error) -> Provider C (success)
```

Each provider has an independent circuit breaker (using gobreaker). When a provider accumulates too many 5xx failures, its circuit opens and the gateway skips it until the recovery interval passes.

Fallback is automatic -- you register multiple models and the gateway handles the rest.

## Token Counting

The gateway counts tokens at two points:

1. **Pre-request estimate:** Before sending to the provider, the gateway estimates input tokens using tiktoken (cl100k_base encoding). This count is checked against the consumer's budget.
2. **Post-response reconciliation:** After the provider responds, the gateway reads the actual token usage from the response and updates the budget with real numbers. If the estimate was higher than actual, the difference is refunded to the budget.

Refunds clamp to zero to prevent negative token counts from over-refunding.

## Token Budgets

Token budgets limit how many tokens or how much cost a consumer or team can use within a time period.

### Create a Budget

```bash
curl -X POST http://localhost:8081/api/v1/ai/budgets \
  -H "Content-Type: application/json" \
  -d '{
    "consumer_id": "CONSUMER_UUID",
    "period": "month",
    "max_tokens": 1000000,
    "max_cost_usd": 100.00,
    "enforcement": "hard"
  }'
```

### Budget Configuration

| Field | Type | Description |
|-------|------|-------------|
| `consumer_id` | UUID | Budget applies to this consumer. |
| `team` | string | Budget applies to this team. At least one of `consumer_id` or `team` is required. |
| `period` | enum | `hour`, `day`, or `month`. Budget resets at the start of each period. |
| `max_tokens` | int | Maximum tokens allowed in the period. |
| `max_cost_usd` | float | Maximum cost in USD allowed in the period. |
| `enforcement` | enum | `hard` (block requests when exceeded) or `soft` (log warnings but allow). |

### Enforcement Behavior

- **Hard enforcement:** When a consumer exceeds their budget, the gateway returns a 429 response with details about the limit.
- **Soft enforcement:** The gateway logs a warning but allows the request through. Useful for monitoring before enforcing.

Budget writes use a detached `context.Background()` with a 5-second timeout so that client disconnects don't prevent usage tracking.

## Prompt and Response Logging

The AI proxy logs request and response metadata to PostgreSQL for usage analytics. PII redaction is applied automatically -- email addresses, phone numbers, SSN patterns, and credit card numbers are stripped from log output using compiled regex patterns before any write.

Logging uses a detached context with a 5-second timeout to survive client disconnects.

## SSE Streaming Details

Streaming works across all three providers:

- The gateway sets response headers and flushes the initial 200 status before the first chunk arrives.
- Anthropic and Gemini event formats are translated to OpenAI-compatible `data: {...}` SSE events inline during streaming, not buffered.
- The final `data: [DONE]` sentinel is sent after the provider stream ends.

SSRF validation is applied to all provider URLs. The gateway resolves DNS hostnames and checks resolved IPs against private ranges before connecting.

## Next Steps

- [Quickstart](../getting-started/quickstart.md) -- set up the gateway from scratch
- [Configuration Reference](../reference/configuration-reference.md) -- AI model and budget schemas
- [Architecture Overview](../architecture/overview.md) -- how the AI proxy fits in the block system
