package ratelimit

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalLimiterStore_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rate       float64
		burst      int
		key        string
		wantCached bool
	}{
		{
			name:       "creates limiter on first access",
			rate:       10.0,
			burst:      20,
			key:        "consumer:abc:route:/api/v1",
			wantCached: false,
		},
		{
			name:       "returns cached limiter on second access",
			rate:       10.0,
			burst:      20,
			key:        "consumer:abc:route:/api/v1",
			wantCached: true,
		},
	}

	store := NewLocalLimiterStore(10.0, 20)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lim := store.Get(tt.key)
			require.NotNil(t, lim)
		})
	}

	// Same key returns same limiter.
	t.Run("same key returns same limiter", func(t *testing.T) {
		t.Parallel()
		s := NewLocalLimiterStore(10.0, 20)
		lim1 := s.Get("key-a")
		lim2 := s.Get("key-a")
		assert.Same(t, lim1, lim2)
	})

	// Different keys return different limiters.
	t.Run("different keys return different limiters", func(t *testing.T) {
		t.Parallel()
		s := NewLocalLimiterStore(10.0, 20)
		lim1 := s.Get("key-a")
		lim2 := s.Get("key-b")
		assert.NotSame(t, lim1, lim2)
	})
}

func TestLocalLimiterStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := NewLocalLimiterStore(10.0, 20)

	var wg sync.WaitGroup
	const goroutines = 100
	const keysPerGoroutine = 10

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < keysPerGoroutine; j++ {
				key := fmt.Sprintf("consumer:%d:route:/api/%d", id%20, j)
				lim := store.Get(key)
				require.NotNil(t, lim)
				// Exercise the limiter.
				lim.Allow()
			}
		}(i)
	}
	wg.Wait()

	// Verify entries were created.
	require.Greater(t, store.Len(), 0)
}

func TestLocalLimiterStore_RespectsBurstAndRate(t *testing.T) {
	t.Parallel()

	// Rate=1, burst=2: only 2 tokens available immediately.
	store := NewLocalLimiterStore(1.0, 2)
	lim := store.Get("test-key")

	// First two should succeed (burst=2).
	assert.True(t, lim.Allow(), "first request should be allowed")
	assert.True(t, lim.Allow(), "second request should be allowed (within burst)")

	// Third should fail (burst exhausted, no time to refill).
	assert.False(t, lim.Allow(), "third request should be denied (burst exhausted)")
}

func TestLocalLimiterStore_GetWithOverride(t *testing.T) {
	t.Parallel()

	store := NewLocalLimiterStore(10.0, 20)

	t.Run("creates limiter with custom rate", func(t *testing.T) {
		t.Parallel()
		lim := store.GetWithOverride("override-key", 1.0, 1)
		require.NotNil(t, lim)
		// Burst=1: only 1 token.
		assert.True(t, lim.Allow())
		assert.False(t, lim.Allow(), "should be limited to burst=1")
	})

	t.Run("updates existing limiter rate", func(t *testing.T) {
		t.Parallel()
		s := NewLocalLimiterStore(100.0, 100)
		// Create with high rate.
		lim1 := s.GetWithOverride("update-key", 100.0, 100)
		require.NotNil(t, lim1)

		// Override with low rate/burst.
		lim2 := s.GetWithOverride("update-key", 1.0, 1)
		// Same limiter instance, just updated.
		assert.Same(t, lim1, lim2)
	})
}

func TestLocalLimiterStore_ShardDistribution(t *testing.T) {
	t.Parallel()

	store := NewLocalLimiterStore(10.0, 20)

	// Insert many keys and verify they distribute across shards.
	for i := 0; i < 1000; i++ {
		store.Get(fmt.Sprintf("key-%d", i))
	}

	// Count non-empty shards.
	nonEmpty := 0
	for i := 0; i < numShards; i++ {
		store.shards[i].mu.RLock()
		if len(store.shards[i].limiters) > 0 {
			nonEmpty++
		}
		store.shards[i].mu.RUnlock()
	}

	// With 1000 keys across 256 shards, we should have a good distribution.
	assert.Greater(t, nonEmpty, 50, "keys should be distributed across many shards")
}

func TestLocalLimiterStore_Len(t *testing.T) {
	t.Parallel()

	store := NewLocalLimiterStore(10.0, 20)
	assert.Equal(t, 0, store.Len())

	store.Get("key-1")
	store.Get("key-2")
	store.Get("key-3")
	assert.Equal(t, 3, store.Len())

	// Accessing existing key should not increase count.
	store.Get("key-1")
	assert.Equal(t, 3, store.Len())
}
