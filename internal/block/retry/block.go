package retry

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/luraproject/lura/v2/proxy"
)

// RetryBlock implements gateway.Block with exponential backoff retry logic,
// crypto/rand jitter, and retry budget enforcement.
type RetryBlock struct {
	config            RetryConfig
	totalRequests     atomic.Int64
	retryingRequests  atomic.Int64
	retryableStatusSet map[int]struct{}
}

// NewRetryBlock creates a new retry block with the given configuration.
func NewRetryBlock(cfg RetryConfig) (*RetryBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating retry block: %w", err)
	}
	cfg.applyDefaults()

	statusSet := make(map[int]struct{}, len(cfg.RetryableStatusCodes))
	for _, code := range cfg.RetryableStatusCodes {
		statusSet[code] = struct{}{}
	}

	return &RetryBlock{
		config:            cfg,
		retryableStatusSet: statusSet,
	}, nil
}

// Name returns the block identifier.
func (b *RetryBlock) Name() string {
	return "retry"
}

// Middleware returns a Lura proxy.Middleware that wraps calls with retry logic.
func (b *RetryBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			b.totalRequests.Add(1)

			// Non-idempotent methods: call once and return.
			if !b.config.RetryNonIdempotent && !isIdempotent(req.Method) {
				return nextProxy(ctx, req)
			}

			// First attempt.
			resp, err := nextProxy(ctx, req)
			if !b.shouldRetry(resp, err) {
				return resp, err
			}

			// Check retry budget before entering retry loop.
			// Budget compares currently-retrying requests against total throughput.
			if !b.canStartRetry() {
				return resp, err
			}

			b.retryingRequests.Add(1)
			defer b.retryingRequests.Add(-1)

			var lastResp *proxy.Response
			var lastErr error
			lastResp = resp
			lastErr = err

			for attempt := 0; attempt < b.config.MaxRetries; attempt++ {
				// Compute backoff with jitter.
				backoff := b.computeBackoff(attempt)

				// Wait with context cancellation support.
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					if lastErr != nil {
						return lastResp, lastErr
					}
					return lastResp, ctx.Err()
				case <-timer.C:
				}

				// Re-check budget before each retry.
				if !b.withinBudget() {
					break
				}

				resp, err = nextProxy(ctx, req)
				lastResp = resp
				lastErr = err

				if !b.shouldRetry(resp, err) {
					return resp, err
				}
			}

			return lastResp, lastErr
		}
	}
}

// shouldRetry returns true if the response or error warrants a retry.
func (b *RetryBlock) shouldRetry(resp *proxy.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp != nil {
		return b.isRetryableStatus(resp.Metadata.StatusCode)
	}
	return false
}

// canStartRetry checks if starting a new retry would stay within budget.
// It uses a prospective check: (currently retrying + 1) / total < budget.
func (b *RetryBlock) canStartRetry() bool {
	total := b.totalRequests.Load()
	if total <= 1 {
		return true // Always allow retry for the first request
	}
	retrying := b.retryingRequests.Load()
	return float64(retrying+1)/float64(total) <= b.config.MaxRetryBudget
}

// withinBudget checks if continuing retries is within the configured budget.
func (b *RetryBlock) withinBudget() bool {
	total := b.totalRequests.Load()
	if total <= 1 {
		return true
	}
	retrying := b.retryingRequests.Load()
	return float64(retrying)/float64(total) <= b.config.MaxRetryBudget
}

// computeBackoff calculates the backoff duration for a given attempt with jitter.
func (b *RetryBlock) computeBackoff(attempt int) time.Duration {
	backoff := b.config.InitialInterval
	for i := 0; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * b.config.Multiplier)
		if backoff > b.config.MaxInterval {
			backoff = b.config.MaxInterval
			break
		}
	}
	jitter := cryptoJitter(backoff / 2)
	return backoff + jitter
}

// cryptoJitter returns a random duration in [0, max) using crypto/rand.
func cryptoJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return max / 2
	}
	return time.Duration(n.Int64())
}

// isRetryableStatus checks if the status code is in the configured retryable set.
func (b *RetryBlock) isRetryableStatus(statusCode int) bool {
	_, ok := b.retryableStatusSet[statusCode]
	return ok
}

// isIdempotent returns true for HTTP methods safe to retry.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodDelete,
		http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
