//go:build integration

package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	return pool
}

func TestDistributedStore_Integration_FullSyncCycle(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// Increment locally.
	for i := 0; i < 42; i++ {
		store.Increment("test-key-sync")
	}

	// Verify local pending.
	assert.Equal(t, int64(42), store.getPending("test-key-sync"))

	// Flush to Postgres.
	store.flushAll(ctx)

	// Pending should be zero after flush.
	assert.Equal(t, int64(0), store.getPending("test-key-sync"))

	// Check should return the count from Postgres.
	count, err := store.Check(ctx, "test-key-sync")
	require.NoError(t, err)
	assert.Equal(t, int64(42), count)
}

func TestDistributedStore_Integration_SlidingWindow(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// Insert a count in the current window.
	store.Increment("window-key")
	store.Increment("window-key")
	store.flushAll(ctx)

	// Insert a count with an old window_start (outside the window).
	_, err := pool.Exec(ctx,
		`INSERT INTO rate_limit_counters (key, window_start, count)
		 VALUES ($1, NOW() - INTERVAL '2 hours', $2)`,
		"window-key", 999,
	)
	require.NoError(t, err)

	// Check should only sum the recent window, not the old row.
	count, err := store.Check(ctx, "window-key")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "sliding window should exclude rows outside window")
}

func TestDistributedStore_Integration_Cleanup(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// Insert old rows that should be cleaned up.
	_, err := pool.Exec(ctx,
		`INSERT INTO rate_limit_counters (key, window_start, count)
		 VALUES ($1, NOW() - INTERVAL '2 hours', $2)`,
		"cleanup-key", 100,
	)
	require.NoError(t, err)

	// Insert a recent row that should NOT be cleaned up.
	store.Increment("cleanup-key")
	store.flushAll(ctx)

	// Run cleanup.
	store.cleanupStale(ctx)

	// Verify old row is deleted but recent row remains.
	var rowCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rate_limit_counters WHERE key = $1`,
		"cleanup-key",
	).Scan(&rowCount)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount, "only recent row should remain after cleanup")
}

func TestDistributedStore_Integration_FlushRestoreOnError(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// Add some counts.
	store.Increment("restore-key")
	store.Increment("restore-key")
	store.Increment("restore-key")

	// Flush with a cancelled context to force failure.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	store.flushAll(cancelledCtx)

	// Pending should be restored after failed flush.
	pending := store.getPending("restore-key")
	assert.Equal(t, int64(3), pending, "pending should be restored after failed flush")
}

func TestDistributedStore_Integration_ConcurrentFlush(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  100000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// 50 goroutines each increment 100 times.
	const goroutines = 50
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				store.Increment("concurrent-key")
			}
		}()
	}
	wg.Wait()

	// Flush and check total.
	store.flushAll(ctx)

	count, err := store.Check(ctx, "concurrent-key")
	require.NoError(t, err)
	assert.Equal(t, int64(goroutines*perGoroutine), count,
		"total after concurrent increments should be exact")
}

func TestDistributedStore_Integration_MultipleFlushesCumulate(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// First batch.
	for i := 0; i < 10; i++ {
		store.Increment("cumulate-key")
	}
	store.flushAll(ctx)

	// Second batch.
	for i := 0; i < 15; i++ {
		store.Increment("cumulate-key")
	}
	store.flushAll(ctx)

	// Total should be 25.
	count, err := store.Check(ctx, "cumulate-key")
	require.NoError(t, err)
	assert.Equal(t, int64(25), count, "multiple flushes should cumulate via ON CONFLICT upsert")
}

func TestDistributedStore_Integration_CheckIncludesPending(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	// Flush 10 to Postgres.
	for i := 0; i < 10; i++ {
		store.Increment("pending-key")
	}
	store.flushAll(ctx)

	// Add 5 more locally (not flushed).
	for i := 0; i < 5; i++ {
		store.Increment("pending-key")
	}

	// Check should return 15 (10 from Postgres + 5 pending).
	count, err := store.Check(ctx, "pending-key")
	require.NoError(t, err)
	assert.Equal(t, int64(15), count, "Check should include both Postgres and local pending")
}

func TestDistributedStore_Integration_MultipleKeys(t *testing.T) {
	pool := setupTestPool(t)
	ctx := context.Background()

	cfg := Config{
		WindowSize:   time.Minute,
		MaxRequests:  1000,
		SyncInterval: 100 * time.Millisecond,
	}
	cfg.applyDefaults()

	store := NewDistributedStore(pool, cfg)

	for i := 0; i < 5; i++ {
		store.Increment(fmt.Sprintf("multi-key-%d", i))
		store.Increment(fmt.Sprintf("multi-key-%d", i))
	}
	store.flushAll(ctx)

	for i := 0; i < 5; i++ {
		count, err := store.Check(ctx, fmt.Sprintf("multi-key-%d", i))
		require.NoError(t, err)
		assert.Equal(t, int64(2), count, "each key should have count=2")
	}
}
