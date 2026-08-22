package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// mockProxy returns a proxy that fails failCount times then succeeds.
func mockProxy(failCount *int, failErr error) proxy.Proxy {
	return func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		if *failCount > 0 {
			*failCount--
			return nil, failErr
		}
		return &proxy.Response{
			Data:       map[string]interface{}{"status": "ok"},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
		}, nil
	}
}

// mockProxyWithStatus returns a proxy that always returns the given status code.
func mockProxyWithStatus(statusCode int) proxy.Proxy {
	return func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			Data:       map[string]interface{}{"status": "error"},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: statusCode},
		}, nil
	}
}

func testConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             "test-service",
		MaxRequests:      1,
		Interval:         5 * time.Millisecond,
		Timeout:          10 * time.Millisecond,
		FailureThreshold: 0.5,
		MinRequests:      2,
	}
}

func TestCircuitBreakerBlock_Name(t *testing.T) {
	t.Parallel()
	cb, err := NewCircuitBreakerBlock(testConfig())
	require.NoError(t, err)
	require.NotNil(t, cb)
	assert.Equal(t, "circuit-breaker", cb.Name())
}

func TestCircuitBreakerBlock_ClosedState_PassesThrough(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	successProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
		}, nil
	}

	p := mw(successProxy)
	resp, err := p(context.Background(), &proxy.Request{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
}

func TestCircuitBreakerBlock_OpensAfterThreshold(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	upstreamErr := errors.New("upstream down")
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return nil, upstreamErr
	}

	p := mw(failProxy)
	ctx := context.Background()

	// Send enough failures to trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Next request should get a 503 ProblemError (circuit open)
	_, err = p(ctx, &proxy.Request{})
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe), "expected ProblemError, got %T: %v", err, err)
	assert.Equal(t, http.StatusServiceUnavailable, pe.StatusCode())
	assert.Contains(t, pe.Type, "circuit_open")
}

func TestCircuitBreakerBlock_OpenReturns503ProblemError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return nil, errors.New("fail")
	}

	p := mw(failProxy)
	ctx := context.Background()

	// Trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Verify the 503 response
	_, err = p(ctx, &proxy.Request{})
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, 503, pe.Status)
	assert.Contains(t, pe.Detail, "circuit breaker open")
}

func TestCircuitBreakerBlock_TransitionsToHalfOpen(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5
	cfg.Timeout = 15 * time.Millisecond

	stateChanges := make(chan string, 10)
	cfg.OnStateChange = func(name string, from, to string) {
		stateChanges <- fmt.Sprintf("%s->%s", from, to)
	}

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()

	callCount := 0
	dynamicProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount++
		// First 3 calls fail, then succeed
		if callCount <= 3 {
			return nil, errors.New("fail")
		}
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
		}, nil
	}

	p := mw(dynamicProxy)
	ctx := context.Background()

	// Trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Verify circuit is open
	_, err = p(ctx, &proxy.Request{})
	require.Error(t, err)

	// Wait for timeout to transition to half-open, then try a test request
	require.Eventually(t, func() bool {
		resp, err := p(ctx, &proxy.Request{})
		return err == nil && resp != nil
	}, 200*time.Millisecond, 5*time.Millisecond, "circuit should transition to half-open and allow test request")
}

func TestCircuitBreakerBlock_HalfOpenSuccessCloses(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5
	cfg.Timeout = 15 * time.Millisecond
	cfg.MaxRequests = 1

	stateChanges := make(chan string, 10)
	cfg.OnStateChange = func(_ string, from, to string) {
		stateChanges <- fmt.Sprintf("%s->%s", from, to)
	}

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()

	callCount := 0
	dynamicProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount++
		if callCount <= 3 {
			return nil, errors.New("fail")
		}
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
		}, nil
	}

	p := mw(dynamicProxy)
	ctx := context.Background()

	// Trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Wait for half-open and succeed
	require.Eventually(t, func() bool {
		resp, err := p(ctx, &proxy.Request{})
		return err == nil && resp != nil
	}, 200*time.Millisecond, 5*time.Millisecond)

	// Verify state went closed->open->half-open->closed
	require.Eventually(t, func() bool {
		// Drain channel and check for closed transition
		for {
			select {
			case change := <-stateChanges:
				if change == "half-open->closed" {
					return true
				}
			default:
				return false
			}
		}
	}, 200*time.Millisecond, 5*time.Millisecond, "should transition from half-open to closed")
}

func TestCircuitBreakerBlock_HalfOpenFailureReopens(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5
	cfg.Timeout = 15 * time.Millisecond
	cfg.MaxRequests = 1

	stateChanges := make(chan string, 10)
	cfg.OnStateChange = func(_ string, from, to string) {
		stateChanges <- fmt.Sprintf("%s->%s", from, to)
	}

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()

	// Always fails
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return nil, errors.New("permanent failure")
	}

	p := mw(failProxy)
	ctx := context.Background()

	// Trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Wait for half-open, attempt request (will fail), should re-open
	require.Eventually(t, func() bool {
		_, _ = p(ctx, &proxy.Request{})
		// Check if we got a half-open->open transition
		for {
			select {
			case change := <-stateChanges:
				if change == "half-open->open" {
					return true
				}
			default:
				return false
			}
		}
	}, 200*time.Millisecond, 5*time.Millisecond, "should transition from half-open back to open")
}

func TestCircuitBreakerBlock_OtherErrorsPassThrough(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 100 // High threshold so breaker never trips

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	upstreamErr := errors.New("specific upstream error")
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return nil, upstreamErr
	}

	p := mw(failProxy)
	_, err = p(context.Background(), &proxy.Request{})
	require.Error(t, err)

	// Should NOT be a ProblemError -- the original error passes through
	var pe *domain.ProblemError
	assert.False(t, errors.As(err, &pe), "non-circuit errors should pass through unchanged")
	assert.True(t, errors.Is(err, upstreamErr))
}

func TestCircuitBreakerBlock_5xxStatusCountsAsFailure(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	p := mw(mockProxyWithStatus(http.StatusBadGateway))
	ctx := context.Background()

	// Send enough 5xx responses to trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Next request should be circuit open
	_, err = p(ctx, &proxy.Request{})
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe), "expected ProblemError after 5xx trips breaker, got %T: %v", err, err)
	assert.Equal(t, http.StatusServiceUnavailable, pe.StatusCode())
}

func TestCircuitBreakerBlock_StateChangeCallback(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MinRequests = 2
	cfg.FailureThreshold = 0.5

	stateChanges := make(chan string, 10)
	cfg.OnStateChange = func(name string, from, to string) {
		stateChanges <- fmt.Sprintf("%s:%s->%s", name, from, to)
	}

	cb, err := NewCircuitBreakerBlock(cfg)
	require.NoError(t, err)

	mw := cb.Middleware()
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return nil, errors.New("fail")
	}

	p := mw(failProxy)
	ctx := context.Background()

	// Trip the breaker
	for i := 0; i < 3; i++ {
		_, _ = p(ctx, &proxy.Request{})
	}

	// Should have received a state change notification
	require.Eventually(t, func() bool {
		select {
		case change := <-stateChanges:
			return change == "test-service:closed->open"
		default:
			// Try one more request to ensure the callback fires
			_, _ = p(ctx, &proxy.Request{})
			return false
		}
	}, 200*time.Millisecond, 5*time.Millisecond, "should receive closed->open state change")
}
