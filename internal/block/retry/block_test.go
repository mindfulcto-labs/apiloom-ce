package retry

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:           3,
		InitialInterval:      1 * time.Millisecond,
		MaxInterval:          10 * time.Millisecond,
		Multiplier:           2.0,
		RetryableStatusCodes: []int{502, 503, 504},
		RetryNonIdempotent:   false,
		MaxRetryBudget:       0.2,
	}
}

func TestRetryBlock_Name(t *testing.T) {
	t.Parallel()
	rb, err := NewRetryBlock(testConfig())
	require.NoError(t, err)
	require.NotNil(t, rb)
	assert.Equal(t, "retry", rb.Name())
}

func TestRetryBlock_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	var callCount atomic.Int32
	successProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount.Add(1)
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
		}, nil
	}

	mw := rb.Middleware()
	p := mw(successProxy)
	resp, err := p(context.Background(), &proxy.Request{Method: http.MethodGet})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
	assert.Equal(t, int32(1), callCount.Load(), "should only call once on success")
}

func TestRetryBlock_RetriesOnRetryableStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"502 Bad Gateway", 502},
		{"503 Service Unavailable", 503},
		{"504 Gateway Timeout", 504},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			cfg.MaxRetries = 2
			rb, err := NewRetryBlock(cfg)
			require.NoError(t, err)

			var callCount atomic.Int32
			failThenSucceed := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				n := callCount.Add(1)
				if n <= 2 {
					return &proxy.Response{
						Data:       map[string]interface{}{"error": true},
						IsComplete: true,
						Metadata:   proxy.Metadata{StatusCode: tt.statusCode},
					}, nil
				}
				return &proxy.Response{
					Data:       map[string]interface{}{"ok": true},
					IsComplete: true,
					Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
				}, nil
			}

			mw := rb.Middleware()
			p := mw(failThenSucceed)
			resp, err := p(context.Background(), &proxy.Request{Method: http.MethodGet})
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
			assert.Equal(t, int32(3), callCount.Load(), "should have retried twice then succeeded")
		})
	}
}

func TestRetryBlock_DoesNotRetryPOST(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.RetryNonIdempotent = false
	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	var callCount atomic.Int32
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount.Add(1)
		return &proxy.Response{
			Metadata: proxy.Metadata{StatusCode: http.StatusBadGateway},
		}, nil
	}

	mw := rb.Middleware()
	p := mw(failProxy)
	resp, err := p(context.Background(), &proxy.Request{Method: http.MethodPost})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(1), callCount.Load(), "POST should not be retried")
}

func TestRetryBlock_DoesNotRetryPATCH(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.RetryNonIdempotent = false
	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	var callCount atomic.Int32
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount.Add(1)
		return &proxy.Response{
			Metadata: proxy.Metadata{StatusCode: http.StatusBadGateway},
		}, nil
	}

	mw := rb.Middleware()
	p := mw(failProxy)
	resp, err := p(context.Background(), &proxy.Request{Method: http.MethodPatch})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(1), callCount.Load(), "PATCH should not be retried")
}

func TestRetryBlock_RetriesIdempotentMethods(t *testing.T) {
	t.Parallel()

	methods := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			cfg.MaxRetries = 1
			rb, err := NewRetryBlock(cfg)
			require.NoError(t, err)

			var callCount atomic.Int32
			failThenSucceed := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				n := callCount.Add(1)
				if n == 1 {
					return &proxy.Response{
						Metadata: proxy.Metadata{StatusCode: http.StatusBadGateway},
					}, nil
				}
				return &proxy.Response{
					Data:       map[string]interface{}{"ok": true},
					IsComplete: true,
					Metadata:   proxy.Metadata{StatusCode: http.StatusOK},
				}, nil
			}

			mw := rb.Middleware()
			p := mw(failThenSucceed)
			resp, err := p(context.Background(), &proxy.Request{Method: method})
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
			assert.Equal(t, int32(2), callCount.Load(), "%s should be retried", method)
		})
	}
}

func TestRetryBlock_BudgetPreventsRetry(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MaxRetries = 3
	cfg.MaxRetryBudget = 0.1 // Very low budget

	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	mw := rb.Middleware()

	var totalCalls atomic.Int32
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		totalCalls.Add(1)
		return &proxy.Response{
			Metadata: proxy.Metadata{StatusCode: http.StatusBadGateway},
		}, nil
	}

	p := mw(failProxy)
	ctx := context.Background()

	// Make multiple requests. With a 10% budget, retries should be limited.
	// First request uses budget, subsequent ones should be denied retries.
	for i := 0; i < 10; i++ {
		_, _ = p(ctx, &proxy.Request{Method: http.MethodGet})
	}

	// With 10% budget, we expect significantly fewer than 10 * 4 = 40 calls
	// (10 initial + up to 30 retries). Budget should cap total retries.
	calls := totalCalls.Load()
	assert.Less(t, calls, int32(40), "budget should limit retries, got %d calls", calls)
}

func TestRetryBlock_ExhaustedRetriesReturnLastError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MaxRetries = 2
	cfg.MaxRetryBudget = 1.0 // No budget limit

	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	lastErr := errors.New("persistent failure")
	var callCount atomic.Int32
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount.Add(1)
		return nil, lastErr
	}

	mw := rb.Middleware()
	p := mw(failProxy)
	_, err = p(context.Background(), &proxy.Request{Method: http.MethodGet})
	require.Error(t, err)
	assert.True(t, errors.Is(err, lastErr), "should return last error, got: %v", err)
	assert.Equal(t, int32(3), callCount.Load(), "1 initial + 2 retries = 3 calls")
}

func TestRetryBlock_ContextCancellationStopsRetry(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MaxRetries = 10
	cfg.InitialInterval = 50 * time.Millisecond
	cfg.MaxRetryBudget = 1.0

	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	var callCount atomic.Int32
	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		callCount.Add(1)
		return &proxy.Response{
			Metadata: proxy.Metadata{StatusCode: http.StatusBadGateway},
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	mw := rb.Middleware()
	p := mw(failProxy)
	_, err = p(ctx, &proxy.Request{Method: http.MethodGet})
	require.Error(t, err)

	// Should have been cut short by context cancellation
	calls := callCount.Load()
	assert.Less(t, calls, int32(10), "context cancellation should stop retries early, got %d", calls)
}

func TestRetryBlock_NoMathRandImport(t *testing.T) {
	t.Parallel()

	// Parse the block.go source file and verify no math/rand import.
	fset := token.NewFileSet()
	dir, err := os.Getwd()
	require.NoError(t, err)

	blockFile := filepath.Join(dir, "block.go")
	f, err := parser.ParseFile(fset, blockFile, nil, parser.ImportsOnly)
	require.NoError(t, err)

	for _, imp := range f.Imports {
		path := ""
		if imp.Path != nil {
			path = imp.Path.Value
		}
		// Check for exact "math/rand" or "math/rand/v2" import
		assert.False(t, path == `"math/rand"` || path == `"math/rand/v2"`,
			"block.go must not import math/rand, found: %s", path)
	}
}

func TestRetryBlock_ExhaustedRetriesReturnLastResponse(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.MaxRetries = 1
	cfg.MaxRetryBudget = 1.0

	rb, err := NewRetryBlock(cfg)
	require.NoError(t, err)

	failProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			Data:       map[string]interface{}{"error": "bad gateway"},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: http.StatusBadGateway},
		}, nil
	}

	mw := rb.Middleware()
	p := mw(failProxy)
	resp, err := p(context.Background(), &proxy.Request{Method: http.MethodGet})

	// When retries are exhausted with retryable status (no error), return the last response
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadGateway, resp.Metadata.StatusCode)
}

func TestRetryBlock_UseCryptoRand(t *testing.T) {
	t.Parallel()

	// Verify crypto/rand is used by checking imports in block.go
	fset := token.NewFileSet()
	dir, err := os.Getwd()
	require.NoError(t, err)

	blockFile := filepath.Join(dir, "block.go")
	f, err := parser.ParseFile(fset, blockFile, nil, 0)
	require.NoError(t, err)

	hasCryptoRand := false
	for _, imp := range f.Imports {
		if imp.Path != nil && strings.Contains(imp.Path.Value, "crypto/rand") {
			hasCryptoRand = true
		}
	}
	assert.True(t, hasCryptoRand, "block.go must import crypto/rand for jitter")

	// Also verify cryptoJitter function exists
	hasCryptoJitter := false
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "cryptoJitter" {
			hasCryptoJitter = true
		}
		return true
	})
	assert.True(t, hasCryptoJitter, "block.go must have a cryptoJitter function")
}
