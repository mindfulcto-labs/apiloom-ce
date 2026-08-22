package gateway

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// --- Mock implementations ---

type mockCanaryConfigGetter struct {
	fn func(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error)
}

func (m *mockCanaryConfigGetter) GetCanary(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error) {
	return m.fn(ctx, routeID)
}

type mockCanaryVersionGetter struct {
	fn func(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error)
}

func (m *mockCanaryVersionGetter) GetAPIVersion(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error) {
	return m.fn(ctx, versionID)
}

func TestCanaryBlock_Name(t *testing.T) {
	t.Parallel()
	b := NewCanaryBlock(nil, nil, nil)
	assert.Equal(t, "canary-routing", b.Name())
}

func TestCanaryBlock_ImplementsBlock(t *testing.T) {
	t.Parallel()
	var _ Block = NewCanaryBlock(nil, nil, nil)
}

func TestCanaryBlock_Passthrough_NilGetters(t *testing.T) {
	t.Parallel()

	b := NewCanaryBlock(nil, nil, nil)
	mw := b.Middleware()

	called := false
	backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		called = true
		return &proxy.Response{IsComplete: true}, nil
	}

	p := mw(backend)
	resp, err := p(context.Background(), &proxy.Request{})
	require.NoError(t, err)
	assert.True(t, called)
	assert.True(t, resp.IsComplete)
}

func TestCanaryBlock_Passthrough_NoConfig(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()

	configGetter := &mockCanaryConfigGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
			return nil, nil // No canary config
		},
	}
	versionGetter := &mockCanaryVersionGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.APIVersion, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}

	b := NewCanaryBlock(configGetter, versionGetter, nil)
	mw := b.Middleware()

	called := false
	backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		called = true
		// No canary version should be in context.
		assert.Nil(t, CanaryVersionFromContext(ctx))
		return &proxy.Response{IsComplete: true}, nil
	}

	p := mw(backend)
	req := &proxy.Request{
		Headers: http.Header{"X-Route-Id": []string{routeID.String()}},
	}
	_, err := p(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestCanaryBlock_WeightDistribution(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	primaryID := uuid.New()
	canaryID := uuid.New()

	configGetter := &mockCanaryConfigGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
			return &domain.CanaryConfig{
				RouteID:          routeID,
				PrimaryVersionID: primaryID,
				CanaryVersionID:  canaryID,
				CanaryWeight:     30, // 30% canary
			}, nil
		},
	}

	versionGetter := &mockCanaryVersionGetter{
		fn: func(_ context.Context, id uuid.UUID) (*domain.APIVersion, error) {
			return &domain.APIVersion{ID: id, VersionTag: "v1.0.0"}, nil
		},
	}

	b := NewCanaryBlock(configGetter, versionGetter, nil)
	mw := b.Middleware()

	canaryCount := 0
	primaryCount := 0
	iterations := 1000

	for i := 0; i < iterations; i++ {
		backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			v := CanaryVersionFromContext(ctx)
			if v != nil && v.ID == canaryID {
				canaryCount++
			} else if v != nil && v.ID == primaryID {
				primaryCount++
			}
			return &proxy.Response{IsComplete: true}, nil
		}

		p := mw(backend)
		req := &proxy.Request{
			Headers: http.Header{"X-Route-Id": []string{routeID.String()}},
		}
		_, err := p(context.Background(), req)
		require.NoError(t, err)
	}

	total := canaryCount + primaryCount
	require.Equal(t, iterations, total, "all requests should be routed")

	// With 30% canary weight over 1000 iterations, expect ~300 canary.
	// Allow a generous margin for randomness (within 10% absolute).
	expectedCanary := float64(iterations) * 0.30
	margin := float64(iterations) * 0.10

	assert.InDelta(t, expectedCanary, float64(canaryCount), margin,
		"canary count should be approximately 30%% of total (got %d/%d)", canaryCount, total)

	expectedPrimary := float64(iterations) * 0.70
	assert.InDelta(t, expectedPrimary, float64(primaryCount), margin,
		"primary count should be approximately 70%% of total (got %d/%d)", primaryCount, total)
}

func TestCanaryBlock_Weight100_AllCanary(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	primaryID := uuid.New()
	canaryID := uuid.New()

	configGetter := &mockCanaryConfigGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
			return &domain.CanaryConfig{
				RouteID:          routeID,
				PrimaryVersionID: primaryID,
				CanaryVersionID:  canaryID,
				CanaryWeight:     100, // 100% canary
			}, nil
		},
	}

	versionGetter := &mockCanaryVersionGetter{
		fn: func(_ context.Context, id uuid.UUID) (*domain.APIVersion, error) {
			return &domain.APIVersion{ID: id}, nil
		},
	}

	b := NewCanaryBlock(configGetter, versionGetter, nil)
	mw := b.Middleware()

	for i := 0; i < 50; i++ {
		backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			v := CanaryVersionFromContext(ctx)
			require.NotNil(t, v)
			assert.Equal(t, canaryID, v.ID)
			return &proxy.Response{IsComplete: true}, nil
		}

		p := mw(backend)
		req := &proxy.Request{
			Headers: http.Header{"X-Route-Id": []string{routeID.String()}},
		}
		_, err := p(context.Background(), req)
		require.NoError(t, err)
	}
}

func TestCanaryBlock_Weight0_AllPrimary(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	primaryID := uuid.New()
	canaryID := uuid.New()

	configGetter := &mockCanaryConfigGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
			return &domain.CanaryConfig{
				RouteID:          routeID,
				PrimaryVersionID: primaryID,
				CanaryVersionID:  canaryID,
				CanaryWeight:     0, // 0% canary = passthrough
			}, nil
		},
	}

	versionGetter := &mockCanaryVersionGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.APIVersion, error) {
			return nil, fmt.Errorf("should not be called for weight 0")
		},
	}

	b := NewCanaryBlock(configGetter, versionGetter, nil)
	mw := b.Middleware()

	called := false
	backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		called = true
		// Weight 0 means passthrough, no version in context.
		assert.Nil(t, CanaryVersionFromContext(ctx))
		return &proxy.Response{IsComplete: true}, nil
	}

	p := mw(backend)
	req := &proxy.Request{
		Headers: http.Header{"X-Route-Id": []string{routeID.String()}},
	}
	_, err := p(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestCanaryBlock_ConfigError_Passthrough(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()

	configGetter := &mockCanaryConfigGetter{
		fn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	b := NewCanaryBlock(configGetter, nil, nil)
	mw := b.Middleware()

	called := false
	backend := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		called = true
		return &proxy.Response{IsComplete: true}, nil
	}

	p := mw(backend)
	req := &proxy.Request{
		Headers: http.Header{"X-Route-Id": []string{routeID.String()}},
	}
	_, err := p(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestCryptoRandIntn(t *testing.T) {
	t.Parallel()

	seen := make(map[int]bool)
	for i := 0; i < 200; i++ {
		n, err := cryptoRandIntn(100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 0)
		assert.Less(t, n, 100)
		seen[n] = true
	}
	// With 200 draws from [0,100), we should see a decent spread.
	assert.Greater(t, len(seen), 20, "expected reasonable distribution")
}

// Suppress unused import warning
var _ = math.Pi
