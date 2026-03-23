package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luraproject/lura/v2/proxy"

	"github.com/cto-venkat/apiloom-ce/internal/block/auth"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// RateLimitBlock implements gateway.Block for local token-bucket rate limiting
// with consumer-aware keying and tier-based defaults. When DistributedEnabled is
// true and a pgxpool.Pool is provided, it also enforces aggregate window limits
// across multiple gateway nodes via Postgres UNLOGGED tables.
type RateLimitBlock struct {
	config      Config
	store       *LocalLimiterStore
	distributed *DistributedStore // nil when DistributedEnabled=false or no pool
}

// NewRateLimitBlock creates a new rate limit block with the given configuration.
// pool may be nil; when non-nil and cfg.DistributedEnabled is true, a
// DistributedStore is created for cross-node rate limiting.
// Returns an error if DistributedEnabled + FailClosed but no pool is provided.
func NewRateLimitBlock(cfg Config, pool ...*pgxpool.Pool) (*RateLimitBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating rate limit block: %w", err)
	}
	cfg.applyDefaults()

	var p *pgxpool.Pool
	if len(pool) > 0 {
		p = pool[0]
	}

	// Fail-closed validation: can't be distributed + fail-closed without Postgres.
	if cfg.DistributedEnabled && p == nil && cfg.FailClosed {
		return nil, fmt.Errorf("creating rate limit block: distributed + fail-closed requires a postgres pool")
	}

	store := NewLocalLimiterStore(cfg.Rate, cfg.Burst)

	var dist *DistributedStore
	if cfg.DistributedEnabled && p != nil {
		dist = NewDistributedStore(p, cfg)
	}

	return &RateLimitBlock{
		config:      cfg,
		store:       store,
		distributed: dist,
	}, nil
}

// Start launches the distributed store's sync and cleanup loops.
// Blocks until ctx is cancelled. No-op if distributed is nil.
func (b *RateLimitBlock) Start(ctx context.Context) error {
	if b.distributed == nil {
		return nil
	}
	return b.distributed.Start(ctx)
}

// Name returns the block identifier.
func (b *RateLimitBlock) Name() string {
	return "rate-limit"
}

// Middleware returns a Lura proxy.Middleware that enforces rate limits per
// consumer or client IP. Returns 429 ProblemError when the limit is exceeded.
func (b *RateLimitBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		// If rate limiting is disabled, pass through directly.
		if !b.config.Enabled {
			return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
				return nextProxy(ctx, req)
			}
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			consumer := auth.GetConsumer(ctx)
			apiKey := auth.GetAPIKey(ctx)
			clientIP := extractClientIP(req)

			key := buildRateLimitKey(consumer, req.Path, clientIP)
			r, burst := resolveRateLimit(consumer, apiKey, b.config)
			lim := b.store.GetWithOverride(key, r, burst)

			// Local token bucket: per-second burst protection.
			if !lim.Allow() {
				return nil, rateLimitError(burst)
			}

			// Distributed check: per-window aggregate limits across nodes.
			// Both local AND distributed must pass for a request to proceed.
			if b.distributed != nil {
				b.distributed.Increment(key)

				globalCount, err := b.distributed.Check(ctx, key)
				if err != nil {
					if b.config.FailClosed {
						// Fail-closed: deny when we can't verify the counter.
						return nil, domain.NewProblemError(
							http.StatusTooManyRequests,
							"rate_limited",
							"rate limit check unavailable",
						)
					}
					// Fail-open fallback: log and allow (local-only).
					slog.Warn("distributed rate limit check failed, falling back to local-only",
						slog.String("key", key),
						slog.String("error", err.Error()),
					)
				} else if globalCount > b.config.MaxRequests && b.config.MaxRequests > 0 {
					return nil, rateLimitError(burst)
				}
			}

			// Call downstream.
			resp, err := nextProxy(ctx, req)
			if err != nil {
				return nil, err
			}

			// Add rate limit headers to successful responses.
			if resp.Metadata.Headers == nil {
				resp.Metadata.Headers = make(map[string][]string)
			}
			resp.Metadata.Headers["X-Ratelimit-Limit"] = []string{strconv.Itoa(burst)}
			remaining := int(math.Max(0, float64(int(lim.Tokens()))))
			resp.Metadata.Headers["X-Ratelimit-Remaining"] = []string{strconv.Itoa(remaining)}
			resetTime := time.Now().Add(time.Second).Unix()
			resp.Metadata.Headers["X-Ratelimit-Reset"] = []string{strconv.FormatInt(resetTime, 10)}

			return resp, nil
		}
	}
}

// rateLimitError builds a 429 ProblemError with Retry-After context.
func rateLimitError(burst int) *domain.ProblemError {
	_ = burst // burst available for future header enrichment
	return domain.NewProblemError(
		http.StatusTooManyRequests,
		"rate_limited",
		"rate limit exceeded",
	)
}

// buildRateLimitKey constructs the rate limit key based on consumer identity
// or client IP as fallback.
func buildRateLimitKey(consumer *domain.Consumer, routePath string, clientIP string) string {
	if consumer != nil {
		return "consumer:" + consumer.ID.String() + ":route:" + routePath
	}
	// Strip port from IP if present.
	host, _, err := net.SplitHostPort(clientIP)
	if err == nil {
		clientIP = host
	}
	return "ip:" + clientIP + ":route:" + routePath
}

// resolveRateLimit determines the rate and burst for a consumer by checking:
// 1. API key RateLimitOverride (highest priority)
// 2. Consumer metadata "plan" field mapped to config.Tiers
// 3. Config default rate/burst (fallback)
func resolveRateLimit(consumer *domain.Consumer, apiKey *domain.APIKey, cfg Config) (float64, int) {
	// 1. Check API key override.
	if apiKey != nil {
		if r, b, ok := parseRateLimitOverride(apiKey.RateLimitOverride); ok {
			return r, b
		}
	}

	// 2. Check consumer tier from metadata.
	if consumer != nil {
		tier := parseTierFromMetadata(consumer.Metadata)
		if tier != "" {
			if tc, ok := cfg.Tiers[tier]; ok {
				return tc.Rate, tc.Burst
			}
		}
	}

	// 3. Default.
	return cfg.Rate, cfg.Burst
}

// parseTierFromMetadata extracts the "plan" field from consumer metadata JSON.
func parseTierFromMetadata(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var m struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(metadata, &m); err != nil {
		return ""
	}
	return m.Plan
}

// parseRateLimitOverride extracts rate/burst from API key override JSON.
// Returns (rate, burst, found). found is false if the override is nil, empty,
// invalid, or missing required fields.
func parseRateLimitOverride(override json.RawMessage) (float64, int, bool) {
	if len(override) == 0 {
		return 0, 0, false
	}
	var o struct {
		Rate  float64 `json:"rate"`
		Burst int     `json:"burst"`
	}
	if err := json.Unmarshal(override, &o); err != nil {
		return 0, 0, false
	}
	if o.Rate <= 0 || o.Burst <= 0 {
		return 0, 0, false
	}
	return o.Rate, o.Burst, true
}

// extractClientIP gets the client IP from request headers, falling back to
// X-Forwarded-For if available.
func extractClientIP(req *proxy.Request) string {
	if xff, ok := req.Headers["X-Forwarded-For"]; ok && len(xff) > 0 && xff[0] != "" {
		return xff[0]
	}
	return "unknown"
}
