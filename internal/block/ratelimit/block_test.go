package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/block/auth"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// successProxy returns a 200 response with empty headers.
func successProxy(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
	return &proxy.Response{
		IsComplete: true,
		Metadata: proxy.Metadata{
			StatusCode: http.StatusOK,
			Headers:    map[string][]string{},
		},
	}, nil
}

func newTestConfig() Config {
	return Config{
		Enabled:    true,
		Rate:       10.0,
		Burst:      2,
		FailClosed: true,
		Tiers: map[string]TierConfig{
			"free":       {Rate: 5.0, Burst: 1, MaxRequests: 100},
			"growth":     {Rate: 50.0, Burst: 10, MaxRequests: 1000},
			"enterprise": {Rate: 500.0, Burst: 100, MaxRequests: 10000},
		},
	}
}

func testRequest() *proxy.Request {
	return &proxy.Request{
		Method:  "GET",
		Path:    "/api/v1/users",
		Headers: map[string][]string{},
	}
}

func invokeBlock(t *testing.T, block *RateLimitBlock, ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
	t.Helper()
	mw := block.Middleware()
	p := mw(successProxy)
	return p(ctx, req)
}

func TestRateLimitBlock_Name(t *testing.T) {
	t.Parallel()

	block, err := NewRateLimitBlock(newTestConfig())
	require.NoError(t, err)
	assert.Equal(t, "rate-limit", block.Name())
}

func TestRateLimitBlock_DisabledPassesThrough(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Enabled = false
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	// Should pass even though we call many times.
	for i := 0; i < 100; i++ {
		resp, err := invokeBlock(t, block, context.Background(), testRequest())
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
	}
}

func TestRateLimitBlock_WithinLimitPassesThrough(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Burst = 5
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	resp, err := invokeBlock(t, block, context.Background(), testRequest())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)

	// Verify rate limit headers on success.
	assert.NotEmpty(t, resp.Metadata.Headers["X-Ratelimit-Limit"])
	assert.NotEmpty(t, resp.Metadata.Headers["X-Ratelimit-Remaining"])
	assert.NotEmpty(t, resp.Metadata.Headers["X-Ratelimit-Reset"])
}

func TestRateLimitBlock_ExceedsLimitReturns429(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Rate = 1.0
	cfg.Burst = 1
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	// First request should pass.
	resp, err := invokeBlock(t, block, context.Background(), testRequest())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)

	// Second request should be rate limited.
	_, err = invokeBlock(t, block, context.Background(), testRequest())
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, http.StatusTooManyRequests, pe.Status)
	assert.Contains(t, pe.Type, "rate_limited")
}

func TestRateLimitBlock_429IncludesHeaders(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Rate = 1.0
	cfg.Burst = 1
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	// Exhaust the burst.
	_, _ = invokeBlock(t, block, context.Background(), testRequest())

	// This should return 429 with headers.
	_, err = invokeBlock(t, block, context.Background(), testRequest())
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, http.StatusTooManyRequests, pe.Status)
}

func TestRateLimitBlock_AuthenticatedConsumerKey(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Rate = 1.0
	cfg.Burst = 1
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	consumerID := uuid.New()
	consumer := &domain.Consumer{
		ID:   consumerID,
		Name: "test-consumer",
	}
	ctx := auth.SetConsumer(context.Background(), consumer)

	// First request passes.
	resp, err := invokeBlock(t, block, ctx, testRequest())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)

	// Second request denied (burst=1).
	_, err = invokeBlock(t, block, ctx, testRequest())
	require.Error(t, err)

	// Different consumer should have separate limit.
	consumer2 := &domain.Consumer{
		ID:   uuid.New(),
		Name: "other-consumer",
	}
	ctx2 := auth.SetConsumer(context.Background(), consumer2)
	resp, err = invokeBlock(t, block, ctx2, testRequest())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)
}

func TestRateLimitBlock_UnauthenticatedUsesIP(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Rate = 1.0
	cfg.Burst = 1
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	req := testRequest()
	req.Headers["X-Forwarded-For"] = []string{"192.168.1.100"}

	// First request passes.
	resp, err := invokeBlock(t, block, context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Metadata.StatusCode)

	// Second from same IP denied.
	_, err = invokeBlock(t, block, context.Background(), req)
	require.Error(t, err)
}

func TestRateLimitBlock_TierResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		metadata      map[string]string
		expectedBurst int
	}{
		{
			name:          "free tier gets low burst",
			metadata:      map[string]string{"plan": "free"},
			expectedBurst: 1, // TierConfig free burst=1
		},
		{
			name:          "growth tier gets higher burst",
			metadata:      map[string]string{"plan": "growth"},
			expectedBurst: 10,
		},
		{
			name:          "enterprise tier gets high burst",
			metadata:      map[string]string{"plan": "enterprise"},
			expectedBurst: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use very low rates so tokens don't refill during the test loop.
			cfg := Config{
				Enabled:    true,
				Rate:       0.001, // Near-zero rate: no refill during test.
				Burst:      2,
				FailClosed: true,
				Tiers: map[string]TierConfig{
					"free":       {Rate: 0.001, Burst: 1, MaxRequests: 100},
					"growth":     {Rate: 0.001, Burst: 10, MaxRequests: 1000},
					"enterprise": {Rate: 0.001, Burst: 100, MaxRequests: 10000},
				},
			}
			block, err := NewRateLimitBlock(cfg)
			require.NoError(t, err)

			meta, _ := json.Marshal(tt.metadata)
			consumer := &domain.Consumer{
				ID:       uuid.New(),
				Name:     "tier-test",
				Metadata: meta,
			}
			ctx := auth.SetConsumer(context.Background(), consumer)

			// Should be able to use up to the tier's burst.
			var allowed int
			for i := 0; i < tt.expectedBurst+5; i++ {
				_, err := invokeBlock(t, block, ctx, testRequest())
				if err == nil {
					allowed++
				}
			}
			assert.Equal(t, tt.expectedBurst, allowed,
				"expected %d requests for tier burst", tt.expectedBurst)
		})
	}
}

func TestRateLimitBlock_APIKeyRateLimitOverride(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	override := map[string]interface{}{
		"rate":  1.0,
		"burst": 3,
	}
	overrideJSON, _ := json.Marshal(override)

	consumer := &domain.Consumer{
		ID:   uuid.New(),
		Name: "override-test",
	}
	apiKey := &domain.APIKey{
		ID:                uuid.New(),
		ConsumerID:        consumer.ID,
		RateLimitOverride: overrideJSON,
	}

	ctx := auth.SetConsumer(context.Background(), consumer)
	ctx = auth.SetAPIKey(ctx, apiKey)

	// Should get burst=3 from override.
	var allowed int
	for i := 0; i < 10; i++ {
		_, err := invokeBlock(t, block, ctx, testRequest())
		if err == nil {
			allowed++
		}
	}
	assert.Equal(t, 3, allowed, "expected 3 requests from API key override burst=3")
}

func TestRateLimitBlock_OverrideTakesPrecedenceOverTier(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	// Consumer has "free" tier (burst=1) but API key overrides to burst=5.
	meta, _ := json.Marshal(map[string]string{"plan": "free"})
	override, _ := json.Marshal(map[string]interface{}{"rate": 100.0, "burst": 5})

	consumer := &domain.Consumer{
		ID:       uuid.New(),
		Name:     "precedence-test",
		Metadata: meta,
	}
	apiKey := &domain.APIKey{
		ID:                uuid.New(),
		ConsumerID:        consumer.ID,
		RateLimitOverride: override,
	}

	ctx := auth.SetConsumer(context.Background(), consumer)
	ctx = auth.SetAPIKey(ctx, apiKey)

	var allowed int
	for i := 0; i < 10; i++ {
		_, err := invokeBlock(t, block, ctx, testRequest())
		if err == nil {
			allowed++
		}
	}
	// Override burst=5 takes precedence over free tier burst=1.
	assert.Equal(t, 5, allowed, "API key override should take precedence over tier")
}

func TestRateLimitBlock_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.Rate = 1.0
	cfg.Burst = 1
	block, err := NewRateLimitBlock(cfg)
	require.NoError(t, err)

	// Exhaust burst.
	_, _ = invokeBlock(t, block, context.Background(), testRequest())

	// Get the 429 error.
	_, err = invokeBlock(t, block, context.Background(), testRequest())
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, http.StatusTooManyRequests, pe.Status)
}

func TestBuildRateLimitKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		consumer *domain.Consumer
		path     string
		clientIP string
		want     string
	}{
		{
			name:     "authenticated consumer",
			consumer: &domain.Consumer{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")},
			path:     "/api/v1/users",
			clientIP: "10.0.0.1",
			want:     "consumer:11111111-1111-1111-1111-111111111111:route:/api/v1/users",
		},
		{
			name:     "unauthenticated uses IP",
			consumer: nil,
			path:     "/api/v1/users",
			clientIP: "192.168.1.100",
			want:     "ip:192.168.1.100:route:/api/v1/users",
		},
		{
			name:     "IP with port stripped",
			consumer: nil,
			path:     "/api/v1",
			clientIP: "192.168.1.100:8080",
			want:     "ip:192.168.1.100:route:/api/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildRateLimitKey(tt.consumer, tt.path, tt.clientIP)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveRateLimit(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()

	tests := []struct {
		name      string
		consumer  *domain.Consumer
		apiKey    *domain.APIKey
		wantRate  float64
		wantBurst int
	}{
		{
			name:      "no consumer uses defaults",
			consumer:  nil,
			apiKey:    nil,
			wantRate:  10.0,
			wantBurst: 2,
		},
		{
			name: "consumer with free tier",
			consumer: &domain.Consumer{
				ID:       uuid.New(),
				Metadata: json.RawMessage(`{"plan":"free"}`),
			},
			apiKey:    nil,
			wantRate:  5.0,
			wantBurst: 1,
		},
		{
			name: "consumer with growth tier",
			consumer: &domain.Consumer{
				ID:       uuid.New(),
				Metadata: json.RawMessage(`{"plan":"growth"}`),
			},
			apiKey:    nil,
			wantRate:  50.0,
			wantBurst: 10,
		},
		{
			name: "API key override takes precedence",
			consumer: &domain.Consumer{
				ID:       uuid.New(),
				Metadata: json.RawMessage(`{"plan":"free"}`),
			},
			apiKey: &domain.APIKey{
				RateLimitOverride: json.RawMessage(`{"rate":100.0,"burst":50}`),
			},
			wantRate:  100.0,
			wantBurst: 50,
		},
		{
			name: "consumer with no tier metadata uses default",
			consumer: &domain.Consumer{
				ID:       uuid.New(),
				Metadata: json.RawMessage(`{}`),
			},
			apiKey:    nil,
			wantRate:  10.0,
			wantBurst: 2,
		},
		{
			name: "consumer with unknown tier uses default",
			consumer: &domain.Consumer{
				ID:       uuid.New(),
				Metadata: json.RawMessage(`{"plan":"platinum"}`),
			},
			apiKey:    nil,
			wantRate:  10.0,
			wantBurst: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, b := resolveRateLimit(tt.consumer, tt.apiKey, cfg)
			assert.Equal(t, tt.wantRate, r)
			assert.Equal(t, tt.wantBurst, b)
		})
	}
}

func TestParseTierFromMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata json.RawMessage
		want     string
	}{
		{name: "valid plan", metadata: json.RawMessage(`{"plan":"free"}`), want: "free"},
		{name: "no plan field", metadata: json.RawMessage(`{"region":"us"}`), want: ""},
		{name: "nil metadata", metadata: nil, want: ""},
		{name: "empty JSON", metadata: json.RawMessage(`{}`), want: ""},
		{name: "invalid JSON", metadata: json.RawMessage(`not-json`), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseTierFromMetadata(tt.metadata))
		})
	}
}

func TestNewRateLimitBlock_DistributedFailClosedRequiresPool(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig()
	cfg.DistributedEnabled = true
	cfg.FailClosed = true

	// No pool provided: should error.
	_, err := NewRateLimitBlock(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "distributed + fail-closed requires a postgres pool")
}

func TestRateLimitBlock_DistributedNilPoolFailClosed(t *testing.T) {
	t.Parallel()

	// distributed store with nil pool should fail-closed via Check error.
	cfg := newTestConfig()
	cfg.DistributedEnabled = true
	cfg.FailClosed = true
	cfg.MaxRequests = 1000

	// Create store directly to simulate the scenario.
	store := NewLocalLimiterStore(cfg.Rate, cfg.Burst)
	dist := NewDistributedStore(nil, cfg) // nil pool

	block := &RateLimitBlock{
		config:      cfg,
		store:       store,
		distributed: dist,
	}

	// First request: local limiter allows, but distributed Check fails (nil pool).
	// FailClosed = true, so should return 429.
	_, err := invokeBlock(t, block, context.Background(), testRequest())
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, http.StatusTooManyRequests, pe.Status)
	assert.Contains(t, pe.Detail, "rate limit check unavailable")
}

func TestRateLimitBlock_StartNoopWithoutDistributed(t *testing.T) {
	t.Parallel()

	block, err := NewRateLimitBlock(newTestConfig())
	require.NoError(t, err)

	// Start should be a no-op when distributed is nil.
	err = block.Start(context.Background())
	require.NoError(t, err)
}

func TestParseRateLimitOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		override  json.RawMessage
		wantRate  float64
		wantBurst int
		wantFound bool
	}{
		{
			name:      "valid override",
			override:  json.RawMessage(`{"rate":100.0,"burst":50}`),
			wantRate:  100.0,
			wantBurst: 50,
			wantFound: true,
		},
		{
			name:      "nil override",
			override:  nil,
			wantRate:  0,
			wantBurst: 0,
			wantFound: false,
		},
		{
			name:      "empty override",
			override:  json.RawMessage(`{}`),
			wantRate:  0,
			wantBurst: 0,
			wantFound: false,
		},
		{
			name:      "invalid JSON",
			override:  json.RawMessage(`not-json`),
			wantRate:  0,
			wantBurst: 0,
			wantFound: false,
		},
		{
			name:      "rate only not enough",
			override:  json.RawMessage(`{"rate":10.0}`),
			wantRate:  0,
			wantBurst: 0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, b, found := parseRateLimitOverride(tt.override)
			assert.Equal(t, tt.wantRate, r)
			assert.Equal(t, tt.wantBurst, b)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}
