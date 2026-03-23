// Package circuitbreaker implements a circuit breaker block for upstream resilience.
package circuitbreaker

import (
	"fmt"
	"time"
)

// CircuitBreakerConfig configures the circuit breaker behavior.
type CircuitBreakerConfig struct {
	// Name identifies the upstream service (e.g., "backend-api").
	Name string

	// MaxRequests is the max number of requests allowed in half-open state.
	MaxRequests uint32

	// Interval is the closed-state cyclic period for clearing counts.
	Interval time.Duration

	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration

	// FailureThreshold is the failure rate (0.0-1.0) that trips the breaker.
	FailureThreshold float64

	// MinRequests is the minimum number of requests before the threshold applies.
	MinRequests uint32

	// OnStateChange is called when the circuit breaker changes state.
	OnStateChange func(name string, from, to string)
}

// Validate checks that the configuration values are within acceptable ranges.
func (c *CircuitBreakerConfig) Validate() error {
	if c.FailureThreshold != 0 && (c.FailureThreshold < 0 || c.FailureThreshold > 1) {
		return fmt.Errorf("validating circuit breaker config: failure threshold must be between 0 and 1, got %f", c.FailureThreshold)
	}
	if c.MinRequests != 0 && c.MinRequests < 1 {
		return fmt.Errorf("validating circuit breaker config: min requests must be >= 1")
	}
	if c.Timeout < 0 {
		return fmt.Errorf("validating circuit breaker config: timeout must be > 0")
	}
	return nil
}

func (c *CircuitBreakerConfig) applyDefaults() {
	if c.MaxRequests == 0 {
		c.MaxRequests = 3
	}
	if c.Interval == 0 {
		c.Interval = 10 * time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = 0.5
	}
	if c.MinRequests == 0 {
		c.MinRequests = 10
	}
}
