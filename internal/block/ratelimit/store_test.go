package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributedStore_IncrementAddsToLocalCounter(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	store.Increment("key-1")
	store.Increment("key-1")
	store.Increment("key-1")

	// Verify local pending count via getPending helper.
	pending := store.getPending("key-1")
	assert.Equal(t, int64(3), pending)
}

func TestDistributedStore_IncrementDifferentKeys(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	store.Increment("key-a")
	store.Increment("key-a")
	store.Increment("key-b")

	assert.Equal(t, int64(2), store.getPending("key-a"))
	assert.Equal(t, int64(1), store.getPending("key-b"))
}

func TestDistributedStore_CheckReturnsErrorWhenPoolNil(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	_, err := store.Check(context.Background(), "key-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres pool is nil")
}

func TestDistributedStore_ConcurrentIncrementSafe(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100000,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	const goroutines = 50
	const incrementsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				store.Increment("shared-key")
			}
		}()
	}
	wg.Wait()

	expected := int64(goroutines * incrementsPerGoroutine)
	assert.Equal(t, expected, store.getPending("shared-key"))
}

func TestDistributedStore_CollectPending(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	store.Increment("key-1")
	store.Increment("key-1")
	store.Increment("key-2")
	store.Increment("key-2")
	store.Increment("key-2")

	pending := store.collectPending()
	assert.Equal(t, int64(2), pending["key-1"])
	assert.Equal(t, int64(3), pending["key-2"])

	// After collection, pending should be zero.
	assert.Equal(t, int64(0), store.getPending("key-1"))
	assert.Equal(t, int64(0), store.getPending("key-2"))
}

func TestDistributedStore_ShardDistribution(t *testing.T) {
	t.Parallel()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100,
		SyncInterval: time.Second,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(nil, cfg)

	for i := 0; i < 500; i++ {
		store.Increment(fmt.Sprintf("key-%d", i))
	}

	// Verify keys are distributed across multiple shards.
	nonEmpty := 0
	for i := 0; i < numShards; i++ {
		store.shards[i].mu.RLock()
		if len(store.shards[i].counters) > 0 {
			nonEmpty++
		}
		store.shards[i].mu.RUnlock()
	}
	assert.Greater(t, nonEmpty, 30, "keys should be distributed across many shards")
}
