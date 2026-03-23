package timeout

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// fakeProxy returns a proxy that blocks until ctx is done or the done channel
// is closed, then returns the given response. Never uses time.Sleep.
func fakeProxy(resp *proxy.Response, err error, done <-chan struct{}) proxy.Proxy {
	return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		if done != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
			}
		}
		return resp, err
	}
}

func TestTimeoutBlock(t *testing.T) {
	t.Parallel()

	okResp := &proxy.Response{
		Data:       map[string]interface{}{"ok": true},
		IsComplete: true,
		Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
	}

	tests := []struct {
		name           string
		config         TimeoutConfig
		proxy          func() (proxy.Proxy, chan struct{})
		ctxTimeout     time.Duration // optional context-level override
		wantStatus     int
		wantErr        bool
		wantProblem    bool
		problemType    string
		ctxOverride    *time.Duration // per-route override via context value
	}{
		{
			name:   "proxy returns within timeout succeeds",
			config: TimeoutConfig{DefaultTimeout: 5 * time.Second},
			proxy: func() (proxy.Proxy, chan struct{}) {
				ch := make(chan struct{})
				close(ch) // immediately available
				return fakeProxy(okResp, nil, ch), ch
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "proxy exceeding timeout returns 504",
			config: TimeoutConfig{DefaultTimeout: 10 * time.Millisecond},
			proxy: func() (proxy.Proxy, chan struct{}) {
				ch := make(chan struct{}) // never closed — simulates slow upstream
				return fakeProxy(okResp, nil, ch), ch
			},
			wantErr:     true,
			wantProblem: true,
			problemType: "urn:flexgateway:error:timeout",
		},
		{
			name:   "default timeout is 30 seconds when zero value",
			config: TimeoutConfig{}, // zero value
		},
		{
			name:   "per-route timeout override is respected",
			config: TimeoutConfig{DefaultTimeout: 10 * time.Millisecond},
			proxy: func() (proxy.Proxy, chan struct{}) {
				ch := make(chan struct{})
				close(ch)
				return fakeProxy(okResp, nil, ch), ch
			},
			ctxOverride: durationPtr(120 * time.Second),
			wantStatus:  http.StatusOK,
		},
		{
			name:   "context cancellation propagates to upstream",
			config: TimeoutConfig{DefaultTimeout: 5 * time.Second},
			proxy: func() (proxy.Proxy, chan struct{}) {
				ch := make(chan struct{}) // blocks forever
				return fakeProxy(okResp, nil, ch), ch
			},
			ctxTimeout:  1 * time.Millisecond,
			wantErr:     true,
			wantProblem: true,
			problemType: "urn:flexgateway:error:timeout",
		},
		{
			name:   "non-timeout error passes through unchanged",
			config: TimeoutConfig{DefaultTimeout: 5 * time.Second},
			proxy: func() (proxy.Proxy, chan struct{}) {
				return fakeProxy(nil, errors.New("connection refused"), nil), nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Special case: just test default value
			if tt.name == "default timeout is 30 seconds when zero value" {
				cfg := tt.config
				cfg.applyDefaults()
				require.Equal(t, 30*time.Second, cfg.DefaultTimeout)
				return
			}

			block, err := NewTimeoutBlock(tt.config)
			require.NoError(t, err)
			require.Equal(t, "timeout", block.Name())

			mw := block.Middleware()
			p, _ := tt.proxy()
			wrapped := mw(p)

			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			if tt.ctxOverride != nil {
				ctx = WithTimeoutOverride(ctx, *tt.ctxOverride)
			}

			req := &proxy.Request{
				Method: http.MethodGet,
			}
			resp, err := wrapped(ctx, req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantProblem {
					var pe *domain.ProblemError
					require.True(t, errors.As(err, &pe), "expected ProblemError, got %T: %v", err, err)
					require.Equal(t, http.StatusGatewayTimeout, pe.StatusCode())
					require.Equal(t, tt.problemType, pe.Type)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, tt.wantStatus, resp.Metadata.StatusCode)
		})
	}
}

func TestTimeoutConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  TimeoutConfig
		wantErr bool
	}{
		{
			name:   "valid config",
			config: TimeoutConfig{DefaultTimeout: 30 * time.Second},
		},
		{
			name:    "negative timeout",
			config:  TimeoutConfig{DefaultTimeout: -1 * time.Second},
			wantErr: true,
		},
		{
			name:    "timeout exceeds 5 minutes",
			config:  TimeoutConfig{DefaultTimeout: 6 * time.Minute},
			wantErr: true,
		},
		{
			name:   "zero timeout uses defaults after apply",
			config: TimeoutConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTimeoutBlock(tt.config)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
