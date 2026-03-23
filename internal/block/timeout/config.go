// Package timeout implements a timeout block for the gateway proxy pipeline.
// It wraps upstream proxy calls with context.WithTimeout, returning a 504
// Gateway Timeout ProblemError when the deadline is exceeded.
package timeout

import (
	"fmt"
	"time"
)

const (
	// defaultTimeout is the timeout applied when no override is configured.
	defaultTimeout = 30 * time.Second
	// maxTimeout is the upper bound for any configured timeout.
	maxTimeout = 5 * time.Minute
)

// TimeoutConfig holds timeout behavior for the timeout block.
type TimeoutConfig struct {
	// DefaultTimeout is the time allowed for an upstream proxy call before
	// returning 504. Zero means use the default (30s).
	DefaultTimeout time.Duration
}

// Validate checks that the timeout configuration is valid.
// Zero is allowed (applyDefaults fills it in). Negative or > 5 minutes is rejected.
func (c *TimeoutConfig) Validate() error {
	if c.DefaultTimeout < 0 {
		return fmt.Errorf("validating timeout config: timeout must be non-negative, got %s", c.DefaultTimeout)
	}
	if c.DefaultTimeout > maxTimeout {
		return fmt.Errorf("validating timeout config: timeout must be <= %s, got %s", maxTimeout, c.DefaultTimeout)
	}
	return nil
}

// applyDefaults fills zero-value fields with safe defaults.
func (c *TimeoutConfig) applyDefaults() {
	if c.DefaultTimeout == 0 {
		c.DefaultTimeout = defaultTimeout
	}
}
