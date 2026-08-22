package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gobreaker "github.com/sony/gobreaker/v2"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// errServerError is returned internally when upstream returns 5xx,
// so gobreaker counts it as a failure.
var errServerError = errors.New("upstream server error")

// CircuitBreakerBlock implements gateway.Block with a 3-state circuit breaker
// using sony/gobreaker/v2.
type CircuitBreakerBlock struct {
	cb *gobreaker.CircuitBreaker[*proxy.Response]
}

// NewCircuitBreakerBlock creates a new circuit breaker block with the given configuration.
func NewCircuitBreakerBlock(cfg CircuitBreakerConfig) (*CircuitBreakerBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating circuit breaker block: %w", err)
	}
	cfg.applyDefaults()

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < uint32(cfg.MinRequests) {
				return false
			}
			failureRate := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRate >= cfg.FailureThreshold
		},
	}

	if cfg.OnStateChange != nil {
		settings.OnStateChange = func(name string, from, to gobreaker.State) {
			cfg.OnStateChange(name, stateString(from), stateString(to))
		}
	}

	cb := gobreaker.NewCircuitBreaker[*proxy.Response](settings)
	return &CircuitBreakerBlock{cb: cb}, nil
}

// Name returns the block identifier.
func (b *CircuitBreakerBlock) Name() string {
	return "circuit-breaker"
}

// Middleware returns a Lura proxy.Middleware that wraps calls with circuit breaker logic.
func (b *CircuitBreakerBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			resp, err := b.cb.Execute(func() (*proxy.Response, error) {
				r, e := nextProxy(ctx, req)
				if e != nil {
					return r, e
				}
				// Count 5xx as failures so gobreaker tracks them.
				if r != nil && r.Metadata.StatusCode >= http.StatusInternalServerError {
					return r, fmt.Errorf("%w: status %d", errServerError, r.Metadata.StatusCode)
				}
				return r, nil
			})

			if err != nil {
				if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
					return nil, domain.NewProblemError(
						http.StatusServiceUnavailable,
						"circuit_open",
						"service temporarily unavailable — circuit breaker open",
					)
				}
				return resp, err
			}

			return resp, nil
		}
	}
}

func stateString(s gobreaker.State) string {
	switch s {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateHalfOpen:
		return "half-open"
	case gobreaker.StateOpen:
		return "open"
	default:
		return "unknown"
	}
}
