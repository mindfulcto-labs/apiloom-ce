package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	auth "github.com/cto-venkat/apiloom-ce/internal/block/auth"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// --- Mock ConsumerStore ---

type mockConsumerStore struct {
	mu               sync.Mutex
	keys             map[string][]domain.APIKey // prefix -> keys
	consumers        map[uuid.UUID]*domain.Consumer
	getKeysCalled    atomic.Int64
	lastUsedCalled   atomic.Int64
	lastUsedIDs      []uuid.UUID
	rotateFunc       func(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error)
	getKeysErr       error
	getConsumerErr   error
	updateLastUsedMu sync.Mutex
}

func newMockStore() *mockConsumerStore {
	return &mockConsumerStore{
		keys:      make(map[string][]domain.APIKey),
		consumers: make(map[uuid.UUID]*domain.Consumer),
	}
}

func (m *mockConsumerStore) GetAPIKeysByPrefix(_ context.Context, prefix string) ([]domain.APIKey, error) {
	m.getKeysCalled.Add(1)
	if m.getKeysErr != nil {
		return nil, m.getKeysErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[prefix], nil
}

func (m *mockConsumerStore) GetConsumer(_ context.Context, id uuid.UUID) (*domain.Consumer, error) {
	if m.getConsumerErr != nil {
		return nil, m.getConsumerErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.consumers[id]
	if !ok {
		return nil, fmt.Errorf("consumer %s: %w", id, domain.ErrNotFound)
	}
	return c, nil
}

func (m *mockConsumerStore) UpdateAPIKeyLastUsed(_ context.Context, id uuid.UUID) error {
	m.lastUsedCalled.Add(1)
	m.updateLastUsedMu.Lock()
	m.lastUsedIDs = append(m.lastUsedIDs, id)
	m.updateLastUsedMu.Unlock()
	return nil
}

func (m *mockConsumerStore) RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error) {
	if m.rotateFunc != nil {
		return m.rotateFunc(ctx, oldKeyID, gracePeriod)
	}
	return "", nil, errors.New("rotate not implemented in mock")
}

func (m *mockConsumerStore) addKey(rawKey string, consumerID uuid.UUID, active bool, deprecated *time.Time, expires *time.Time) domain.APIKey {
	return m.addKeyWithPermissions(rawKey, consumerID, active, deprecated, expires, nil)
}

func (m *mockConsumerStore) addKeyWithPermissions(rawKey string, consumerID uuid.UUID, active bool, deprecated *time.Time, expires *time.Time, permissions json.RawMessage) domain.APIKey {
	hash, _ := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost) // MinCost for test speed
	prefix := rawKey[:8]
	ak := domain.APIKey{
		ID:           uuid.New(),
		ConsumerID:   consumerID,
		KeyHash:      string(hash),
		KeyPrefix:    prefix,
		IsActive:     active,
		DeprecatedAt: deprecated,
		ExpiresAt:    expires,
		Permissions:  permissions,
		CreatedAt:    time.Now(),
	}
	m.mu.Lock()
	m.keys[prefix] = append(m.keys[prefix], ak)
	m.mu.Unlock()
	return ak
}

func (m *mockConsumerStore) addConsumer(id uuid.UUID, name string) *domain.Consumer {
	c := &domain.Consumer{
		ID:        id,
		Name:      name,
		Metadata:  json.RawMessage(`{}`),
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.mu.Lock()
	m.consumers[id] = c
	m.mu.Unlock()
	return c
}

// --- Helper to invoke middleware ---

func invokeMiddleware(t *testing.T, block *APIKeyBlock, req *proxy.Request) (*proxy.Response, error) {
	t.Helper()
	mw := block.Middleware()
	require.NotNil(t, mw)
	next := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{IsComplete: true}, nil
	}
	p := mw(next)
	return p(context.Background(), req)
}

// invokeMiddlewareWithCtx returns context after middleware runs.
func invokeMiddlewareWithCtx(t *testing.T, block *APIKeyBlock, req *proxy.Request) (context.Context, *proxy.Response, error) {
	t.Helper()
	return invokeMiddlewareWithInputCtx(t, block, context.Background(), req)
}

// invokeMiddlewareWithInputCtx runs middleware with a caller-provided context.
func invokeMiddlewareWithInputCtx(t *testing.T, block *APIKeyBlock, ctx context.Context, req *proxy.Request) (context.Context, *proxy.Response, error) {
	t.Helper()
	mw := block.Middleware()
	require.NotNil(t, mw)

	var capturedCtx context.Context
	next := func(c context.Context, _ *proxy.Request) (*proxy.Response, error) {
		capturedCtx = c
		return &proxy.Response{IsComplete: true}, nil
	}
	p := mw(next)
	resp, err := p(ctx, req)
	return capturedCtx, resp, err
}

func makeRequest(header, value string) *proxy.Request {
	return &proxy.Request{
		Method:  "GET",
		URL:     &url.URL{Path: "/test"},
		Query:   url.Values{},
		Path:    "/test",
		Headers: map[string][]string{header: {value}},
	}
}

func makeQueryRequest(key string) *proxy.Request {
	q := url.Values{"api_key": {key}}
	return &proxy.Request{
		Method:  "GET",
		URL:     &url.URL{Path: "/test", RawQuery: q.Encode()},
		Query:   q,
		Path:    "/test",
		Headers: map[string][]string{},
	}
}

// --- Config Tests ---

func TestAPIKeyConfig(t *testing.T) {
	t.Parallel()

	t.Run("empty HeaderName defaults to X-API-Key", func(t *testing.T) {
		t.Parallel()
		cfg := APIKeyConfig{}
		cfg.Validate()
		assert.Equal(t, "X-API-Key", cfg.HeaderName)
	})

	t.Run("zero CacheTTL defaults to 60s", func(t *testing.T) {
		t.Parallel()
		cfg := APIKeyConfig{}
		cfg.Validate()
		assert.Equal(t, 60*time.Second, cfg.CacheTTL)
	})

	t.Run("custom values preserved", func(t *testing.T) {
		t.Parallel()
		cfg := APIKeyConfig{
			HeaderName: "Authorization",
			CacheTTL:   30 * time.Second,
		}
		cfg.Validate()
		assert.Equal(t, "Authorization", cfg.HeaderName)
		assert.Equal(t, 30*time.Second, cfg.CacheTTL)
	})
}

// --- Block Interface Tests ---

func TestAPIKeyBlockInterface(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	block, err := NewAPIKeyBlock(store, APIKeyConfig{})
	require.NoError(t, err)
	require.NotNil(t, block)
	t.Cleanup(func() { _ = block.Stop() })

	t.Run("Name returns auth-apikey", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "auth-apikey", block.Name())
	})

	t.Run("Middleware returns non-nil", func(t *testing.T) {
		t.Parallel()
		mw := block.Middleware()
		assert.NotNil(t, mw)
	})
}

// --- Key Extraction Tests ---

func TestKeyExtraction(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	consumerID := uuid.New()
	rawKey := "abcdef01234567890123456789abcdef0123456789abcdef0123456789abcdef"
	store.addKey(rawKey, consumerID, true, nil, nil)
	store.addConsumer(consumerID, "test-consumer")

	t.Run("extracts key from X-API-Key header", func(t *testing.T) {
		t.Parallel()
		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)
	})

	t.Run("extracts key from query param when AllowQueryParam", func(t *testing.T) {
		t.Parallel()
		block, err := NewAPIKeyBlock(store, APIKeyConfig{AllowQueryParam: true})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeQueryRequest(rawKey)
		_, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)
	})

	t.Run("no key returns 401 missing_api_key", func(t *testing.T) {
		t.Parallel()
		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := &proxy.Request{
			Method:  "GET",
			URL:     &url.URL{Path: "/test"},
			Query:   url.Values{},
			Path:    "/test",
			Headers: map[string][]string{},
		}
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
		assert.Contains(t, pe.Type, "missing_api_key")
	})

	t.Run("header takes precedence over query param", func(t *testing.T) {
		t.Parallel()
		block, err := NewAPIKeyBlock(store, APIKeyConfig{AllowQueryParam: true})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		// Header has the valid key, query has garbage
		q := url.Values{"api_key": {"invalidgarbage_key_that_wont_match_anything_ever_00000000000"}}
		req := &proxy.Request{
			Method:  "GET",
			URL:     &url.URL{Path: "/test", RawQuery: q.Encode()},
			Query:   q,
			Path:    "/test",
			Headers: map[string][]string{"X-API-Key": {rawKey}},
		}
		_, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)
	})
}

// --- Key Validation Tests ---

func TestKeyValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid key authenticates and sets consumer in context", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "abcdef01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		consumer := store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		ctx, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)

		got := auth.GetConsumer(ctx)
		require.NotNil(t, got)
		assert.Equal(t, consumer.ID, got.ID)
		assert.Equal(t, consumer.Name, got.Name)
	})

	t.Run("invalid key returns 401 invalid_api_key", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "abcdef01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		// Same prefix but different key body
		wrongKey := "abcdef01ffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		req := makeRequest("X-API-Key", wrongKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
		assert.Contains(t, pe.Type, "invalid_api_key")
	})

	t.Run("revoked key returns 403 key_revoked", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "revoked1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, false, nil, nil) // is_active=false
		store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 403, pe.Status)
		assert.Contains(t, pe.Type, "key_revoked")
	})

	t.Run("expired key returns 401 key_expired", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "expired1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		pastTime := time.Now().Add(-1 * time.Hour)
		store.addKey(rawKey, consumerID, true, nil, &pastTime)
		store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
		assert.Contains(t, pe.Type, "key_expired")
	})

	t.Run("key not found returns 401", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		unknownKey := "unknown1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		req := makeRequest("X-API-Key", unknownKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
	})
}

// --- Cache Behavior Tests ---

func TestCacheBehavior(t *testing.T) {
	t.Parallel()

	t.Run("second request within TTL skips store call", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "cached01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{CacheTTL: 5 * time.Second})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)

		// First call
		_, _, err = invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		firstCount := store.getKeysCalled.Load()
		assert.Equal(t, int64(1), firstCount)

		// Second call within TTL -- should NOT hit store
		_, _, err = invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		secondCount := store.getKeysCalled.Load()
		assert.Equal(t, int64(1), secondCount, "store should not be called again within TTL")
	})

	t.Run("request after TTL expiry calls store again", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "ttlexp01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "test-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{CacheTTL: 50 * time.Millisecond})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)

		// First call
		_, _, err = invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.Equal(t, int64(1), store.getKeysCalled.Load())

		// Wait for TTL to expire
		require.Eventually(t, func() bool {
			// Make request and check if store was called again
			_, _, err = invokeMiddlewareWithCtx(t, block, req)
			return store.getKeysCalled.Load() > 1
		}, 2*time.Second, 60*time.Millisecond, "store should be called again after TTL expiry")
	})
}

// --- Consumer Context Tests ---

func TestConsumerContext(t *testing.T) {
	t.Parallel()

	t.Run("consumer ID matches key consumer_id", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "ctxtest1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "ctx-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		ctx, _, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)

		got := auth.GetConsumer(ctx)
		require.NotNil(t, got)
		assert.Equal(t, consumerID, got.ID)
	})
}

// --- Security Tests ---

func TestSecurity(t *testing.T) {
	t.Parallel()

	t.Run("bcrypt.CompareHashAndPassword used for validation", func(t *testing.T) {
		// This test verifies the block works with bcrypt-hashed keys.
		// The mock store returns bcrypt hashes; if the block uses ==, it will fail.
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "bcrypt01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "bcrypt-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)
	})

	t.Run("lastUsedCh dispatches update without blocking", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "lastuse1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		ak := store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "lastused-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)

		req := makeRequest("X-API-Key", rawKey)
		_, _, err = invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)

		// Stop to drain the channel
		err = block.Stop()
		require.NoError(t, err)

		// Verify UpdateAPIKeyLastUsed was called with the key ID
		require.Eventually(t, func() bool {
			return store.lastUsedCalled.Load() >= 1
		}, 2*time.Second, 10*time.Millisecond)

		store.updateLastUsedMu.Lock()
		found := false
		for _, id := range store.lastUsedIDs {
			if id == ak.ID {
				found = true
				break
			}
		}
		store.updateLastUsedMu.Unlock()
		assert.True(t, found, "expected UpdateAPIKeyLastUsed to be called with key ID %s", ak.ID)
	})

	t.Run("api_key query parameter stripped before forwarding", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "qstrip01234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		store.addConsumer(consumerID, "strip-consumer")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{AllowQueryParam: true})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		mw := block.Middleware()
		require.NotNil(t, mw)

		var forwardedQuery url.Values
		next := func(_ context.Context, r *proxy.Request) (*proxy.Response, error) {
			forwardedQuery = r.Query
			return &proxy.Response{IsComplete: true}, nil
		}

		q := url.Values{"api_key": {rawKey}, "other": {"keep"}}
		req := &proxy.Request{
			Method:  "GET",
			URL:     &url.URL{Path: "/test", RawQuery: q.Encode()},
			Query:   q,
			Path:    "/test",
			Headers: map[string][]string{},
		}

		p := mw(next)
		_, err = p(context.Background(), req)
		require.NoError(t, err)

		assert.Empty(t, forwardedQuery.Get("api_key"), "api_key should be stripped from query")
		assert.Equal(t, "keep", forwardedQuery.Get("other"), "other params should be preserved")
	})
}

// --- Key Rotation Tests ---

func TestKeyRotation(t *testing.T) {
	t.Parallel()

	t.Run("deprecated key within grace period still works", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "deprkey1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		deprecatedAt := time.Now().Add(-5 * time.Minute) // deprecated 5 min ago
		store.addKey(rawKey, consumerID, true, &deprecatedAt, nil)
		store.addConsumer(consumerID, "rotate-consumer")

		// Grace period is 1 hour, so the key should still work
		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, resp, err := invokeMiddlewareWithCtx(t, block, req)
		require.NoError(t, err)
		assert.True(t, resp.IsComplete)
	})

	t.Run("deprecated key after grace period returns 401", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "expdepr1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		deprecatedAt := time.Now().Add(-2 * time.Hour) // deprecated 2 hours ago
		store.addKey(rawKey, consumerID, true, &deprecatedAt, nil)
		store.addConsumer(consumerID, "rotate-consumer-2")

		// Default grace period is 1 hour, key should be rejected
		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
	})
}

// --- Store Error Handling ---

func TestStoreErrors(t *testing.T) {
	t.Parallel()

	t.Run("store error fails closed with 401", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		store.getKeysErr = errors.New("database unavailable")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", "anykey01234567890123456789abcdef0123456789abcdef0123456789abcdef")
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
	})

	t.Run("consumer store GetConsumer error fails closed", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()
		consumerID := uuid.New()
		rawKey := "conserr1234567890123456789abcdef0123456789abcdef0123456789abcdef"
		store.addKey(rawKey, consumerID, true, nil, nil)
		// Do NOT add consumer -- GetConsumer will return ErrNotFound
		store.getConsumerErr = errors.New("db connection lost")

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", rawKey)
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
	})

	t.Run("short key returns 401", func(t *testing.T) {
		t.Parallel()
		store := newMockStore()

		block, err := NewAPIKeyBlock(store, APIKeyConfig{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = block.Stop() })

		req := makeRequest("X-API-Key", "short")
		_, err = invokeMiddleware(t, block, req)
		require.Error(t, err)
		var pe *domain.ProblemError
		require.True(t, errors.As(err, &pe))
		assert.Equal(t, 401, pe.Status)
	})
}

// --- Scope Enforcement Tests ---

func TestScopeEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permissions json.RawMessage
		routeAPIID  string
		method      string
		wantAllow   bool
		wantStatus  int
	}{
		{
			name:        "unrestricted key with route API ID allowed",
			permissions: nil,
			routeAPIID:  "payments",
			method:      "GET",
			wantAllow:   true,
		},
		{
			name:        "restricted key with matching read scope allowed",
			permissions: json.RawMessage(`["api:payments:read"]`),
			routeAPIID:  "payments",
			method:      "GET",
			wantAllow:   true,
		},
		{
			name:        "restricted key with matching write scope allowed for POST",
			permissions: json.RawMessage(`["api:payments:write"]`),
			routeAPIID:  "payments",
			method:      "POST",
			wantAllow:   true,
		},
		{
			name:        "restricted key without matching scope returns 403",
			permissions: json.RawMessage(`["api:catalog:read"]`),
			routeAPIID:  "payments",
			method:      "GET",
			wantAllow:   false,
			wantStatus:  403,
		},
		{
			name:        "restricted key with wildcard API scope allowed",
			permissions: json.RawMessage(`["api:*:read"]`),
			routeAPIID:  "payments",
			method:      "GET",
			wantAllow:   true,
		},
		{
			name:        "restricted key with wildcard action scope allowed",
			permissions: json.RawMessage(`["api:payments:*"]`),
			routeAPIID:  "payments",
			method:      "DELETE",
			wantAllow:   true,
		},
		{
			name:        "no route API ID in context skips scope check",
			permissions: json.RawMessage(`["api:catalog:read"]`),
			routeAPIID:  "",
			method:      "GET",
			wantAllow:   true,
		},
		{
			name:        "restricted key read scope blocks write method",
			permissions: json.RawMessage(`["api:payments:read"]`),
			routeAPIID:  "payments",
			method:      "POST",
			wantAllow:   false,
			wantStatus:  403,
		},
		{
			name:        "HEAD method maps to read",
			permissions: json.RawMessage(`["api:payments:read"]`),
			routeAPIID:  "payments",
			method:      "HEAD",
			wantAllow:   true,
		},
		{
			name:        "OPTIONS method maps to read",
			permissions: json.RawMessage(`["api:payments:read"]`),
			routeAPIID:  "payments",
			method:      "OPTIONS",
			wantAllow:   true,
		},
		{
			name:        "PUT method maps to write",
			permissions: json.RawMessage(`["api:payments:write"]`),
			routeAPIID:  "payments",
			method:      "PUT",
			wantAllow:   true,
		},
		{
			name:        "PATCH method maps to write",
			permissions: json.RawMessage(`["api:payments:write"]`),
			routeAPIID:  "payments",
			method:      "PATCH",
			wantAllow:   true,
		},
		{
			name:        "DELETE method maps to write",
			permissions: json.RawMessage(`["api:payments:write"]`),
			routeAPIID:  "payments",
			method:      "DELETE",
			wantAllow:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newMockStore()
			consumerID := uuid.New()
			// Use a unique raw key per test to avoid cache collisions.
			rawKey := fmt.Sprintf("scope%s%s%02d0123456789abcdef0123456789abcdef0123456789abcdef01",
				tt.routeAPIID, tt.method, len(tt.permissions))
			// Pad or truncate to exactly 64 chars.
			for len(rawKey) < 64 {
				rawKey += "0"
			}
			rawKey = rawKey[:64]

			store.addKeyWithPermissions(rawKey, consumerID, true, nil, nil, tt.permissions)
			store.addConsumer(consumerID, "scope-test-consumer")

			block, err := NewAPIKeyBlock(store, APIKeyConfig{CacheTTL: 50 * time.Millisecond})
			require.NoError(t, err)
			t.Cleanup(func() { _ = block.Stop() })

			req := &proxy.Request{
				Method:  tt.method,
				URL:     &url.URL{Path: "/test"},
				Query:   url.Values{},
				Path:    "/test",
				Headers: map[string][]string{"X-API-Key": {rawKey}},
			}

			ctx := context.Background()
			if tt.routeAPIID != "" {
				ctx = auth.SetRouteAPIID(ctx, tt.routeAPIID)
			}

			_, resp, err := invokeMiddlewareWithInputCtx(t, block, ctx, req)
			if tt.wantAllow {
				require.NoError(t, err)
				assert.True(t, resp.IsComplete)
			} else {
				require.Error(t, err)
				var pe *domain.ProblemError
				require.True(t, errors.As(err, &pe))
				assert.Equal(t, tt.wantStatus, pe.Status)
			}
		})
	}
}

func TestRedactKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "normal key", key: "abcdef0123456789", want: "...6789"},
		{name: "short key", key: "ab", want: "****"},
		{name: "exactly 4 chars", key: "abcd", want: "****"},
		{name: "5 chars", key: "abcde", want: "...bcde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, redactKey(tt.key))
		})
	}
}
