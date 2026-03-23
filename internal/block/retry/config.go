// Package retry implements a retry block with exponential backoff and crypto/rand jitter.
package retry

import (
	"fmt"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of additional attempts after the first failure.
	MaxRetries int

	// InitialInterval is the base backoff duration before the first retry.
	InitialInterval time.Duration

	// MaxInterval is the upper bound for backoff duration.
	MaxInterval time.Duration

	// Multiplier is the exponential backoff multiplier.
	Multiplier float64

	// RetryableStatusCodes are HTTP status codes that trigger a retry.
	RetryableStatusCodes []int

	// RetryNonIdempotent controls whether POST/PATCH are retried.
	// Default false means POST/PATCH are NOT retried.
	RetryNonIdempotent bool

	// MaxRetryBudget is the max fraction of total requests that can be retries (0.0-1.0).
	MaxRetryBudget float64
}

// Validate checks that configuration values are within acceptable ranges.
func (c *RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return fmt.Errorf("validating retry config: max retries must be >= 0, got %d", c.MaxRetries)
	}
	if c.InitialInterval < 0 {
		return fmt.Errorf("validating retry config: initial interval must be > 0")
	}
	if c.Multiplier != 0 && c.Multiplier <= 1 {
		return fmt.Errorf("validating retry config: multiplier must be > 1, got %f", c.Multiplier)
	}
	if c.MaxRetryBudget < 0 || c.MaxRetryBudget > 1 {
		return fmt.Errorf("validating retry config: max retry budget must be between 0 and 1, got %f", c.MaxRetryBudget)
	}
	return nil
}

func (c *RetryConfig) applyDefaults() {
	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.InitialInterval == 0 {
		c.InitialInterval = 100 * time.Millisecond
	}
	if c.MaxInterval == 0 {
		c.MaxInterval = 5 * time.Second
	}
	if c.Multiplier == 0 {
		c.Multiplier = 2.0
	}
	if len(c.RetryableStatusCodes) == 0 {
		c.RetryableStatusCodes = []int{502, 503, 504}
	}
	if c.MaxRetryBudget == 0 {
		c.MaxRetryBudget = 0.2
	}
}
