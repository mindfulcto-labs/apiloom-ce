// Package ratelimit implements rate limiting blocks for the gateway proxy pipeline.
// This file defines configuration types for local (token bucket), distributed
// (Postgres sliding window), and per-consumer tiered rate limiting.
package ratelimit

import (
	"fmt"
	"time"
)

const (
	defaultRate         = 10.0
	defaultBurst        = 20
	defaultWindowSize   = 1 * time.Minute
	defaultSyncInterval = 250 * time.Millisecond
	minSyncInterval     = 50 * time.Millisecond
)

// Config holds the top-level rate limiting configuration.
type Config struct {
	// Enabled controls whether the rate limit block is active.
	Enabled bool

	// Rate is the sustained requests-per-second for the local token bucket limiter.
	Rate float64

	// Burst is the maximum burst size for the local token bucket.
	Burst int

	// WindowSize is the sliding window duration for distributed rate limiting.
	WindowSize time.Duration

	// MaxRequests is the maximum number of requests allowed per window
	// in the distributed (Postgres-backed) sliding window limiter.
	MaxRequests int64

	// SyncInterval is how often local counters sync to Postgres for the
	// distributed limiter. Minimum 50ms.
	SyncInterval time.Duration

	// DistributedEnabled enables Postgres-backed distributed rate limiting
	// across multiple gateway instances.
	DistributedEnabled bool

	// FailClosed controls behavior when Postgres is unreachable.
	// When true (default), requests are denied (429) if the distributed
	// limiter cannot verify the counter. Per security.md: fail-closed is mandatory.
	FailClosed bool

	// Tiers maps tier names (e.g., "free", "growth", "enterprise") to their
	// rate limit configurations. Used for per-consumer tiered limits.
	Tiers map[string]TierConfig
}

// TierConfig holds rate limit parameters for a specific consumer tier.
type TierConfig struct {
	// Rate is the sustained requests-per-second for this tier.
	Rate float64

	// Burst is the maximum burst size for this tier.
	Burst int

	// MaxRequests is the maximum requests per window for this tier
	// in the distributed sliding window limiter.
	MaxRequests int64
}

// RouteRateLimitConfig holds per-route overrides for rate limiting.
// Pointer fields allow distinguishing "not set" (nil) from zero values.
// Nil means inherit from the global Config.
type RouteRateLimitConfig struct {
	// Enabled overrides whether rate limiting is active for this route.
	// Nil means inherit from global config.
	Enabled *bool

	// Rate overrides the requests-per-second for this route.
	// Nil means inherit from global config.
	Rate *float64

	// Burst overrides the burst size for this route.
	// Nil means inherit from global config.
	Burst *int
}

// Validate checks the rate limit configuration for invalid values.
// Zero values are allowed and filled by applyDefaults.
func (c *Config) Validate() error {
	if c.Rate < 0 {
		return fmt.Errorf("validating rate limit config: rate must be non-negative, got %f", c.Rate)
	}
	if c.Burst < 0 {
		return fmt.Errorf("validating rate limit config: burst must be non-negative, got %d", c.Burst)
	}
	if c.WindowSize < 0 {
		return fmt.Errorf("validating rate limit config: window_size must be non-negative, got %s", c.WindowSize)
	}
	if c.MaxRequests < 0 {
		return fmt.Errorf("validating rate limit config: max_requests must be non-negative, got %d", c.MaxRequests)
	}
	if c.SyncInterval != 0 && c.SyncInterval < minSyncInterval {
		return fmt.Errorf("validating rate limit config: sync_interval must be >= %s, got %s", minSyncInterval, c.SyncInterval)
	}

	for name, tier := range c.Tiers {
		if tier.Rate <= 0 {
			return fmt.Errorf("validating rate limit config: tier %q rate must be > 0, got %f", name, tier.Rate)
		}
		if tier.Burst <= 0 {
			return fmt.Errorf("validating rate limit config: tier %q burst must be > 0, got %d", name, tier.Burst)
		}
	}

	return nil
}

// applyDefaults fills zero-value fields with safe defaults.
func (c *Config) applyDefaults() {
	if c.Rate == 0 {
		c.Rate = defaultRate
	}
	if c.Burst == 0 {
		c.Burst = defaultBurst
	}
	if c.WindowSize == 0 {
		c.WindowSize = defaultWindowSize
	}
	if c.SyncInterval == 0 {
		c.SyncInterval = defaultSyncInterval
	}
	// FailClosed defaults to true (security.md: fail-closed is mandatory).
	// Since bool zero value is false, we always set it unless explicitly
	// configured. The caller must set FailClosed=false to override.
	// For the default path (new config), we force true.
	if !c.FailClosed {
		c.FailClosed = true
	}
}
