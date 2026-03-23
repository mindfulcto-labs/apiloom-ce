package timeout

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// ctxKey is an unexported type for context keys in this package.
type ctxKey struct{}

// timeoutOverrideKey is the context key for per-route timeout overrides.
var timeoutOverrideKey = ctxKey{}

// WithTimeoutOverride returns a context carrying a per-route timeout override.
// The timeout block reads this value to override the default timeout.
func WithTimeoutOverride(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, timeoutOverrideKey, d)
}

// GetTimeoutOverride retrieves a per-route timeout override from context.
// Returns zero duration and false if no override is set.
func GetTimeoutOverride(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(timeoutOverrideKey).(time.Duration)
	return v, ok
}

// TimeoutBlock implements the Block interface for upstream request timeouts.
type TimeoutBlock struct {
	config TimeoutConfig
}

// NewTimeoutBlock creates a new TimeoutBlock with the given configuration.
// Returns an error if the configuration is invalid.
func NewTimeoutBlock(cfg TimeoutConfig) (*TimeoutBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating timeout block: %w", err)
	}

	cfg.applyDefaults()

	return &TimeoutBlock{
		config: cfg,
	}, nil
}

// Name returns the block identifier.
func (b *TimeoutBlock) Name() string {
	return "timeout"
}

// Middleware returns a Lura proxy.Middleware that wraps upstream calls with
// context.WithTimeout. On deadline exceeded, it returns a 504 ProblemError.
func (b *TimeoutBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			timeout := b.config.DefaultTimeout

			// Check for per-route override in context.
			if override, ok := GetTimeoutOverride(ctx); ok && override > 0 {
				timeout = override
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			resp, err := nextProxy(ctx, req)
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return nil, domain.NewProblemError(
						http.StatusGatewayTimeout,
						"timeout",
						"upstream request timed out",
					)
				}
				return nil, err
			}

			return resp, nil
		}
	}
}
