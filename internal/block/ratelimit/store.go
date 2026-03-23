package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/cto-venkat/apiloom-ce/internal/config"
)

// localCounter holds per-key atomic counters for the distributed store.
// pending accumulates increments since the last flush to Postgres.
// lastKnownTotal holds the most recent total from a Postgres sliding window query.
type localCounter struct {
	pending        atomic.Int64
	lastKnownTotal atomic.Int64
}

// counterShard holds a subset of localCounters protected by a read-write mutex.
// This follows concurrency.md: sharded maps with per-shard mutexes instead of
// sync.Map for rate limit counters.
type counterShard struct {
	mu       sync.RWMutex
	counters map[string]*localCounter
}

// DistributedStore provides distributed rate limiting by maintaining local
// atomic counters that periodically sync to a Postgres UNLOGGED table.
// Sliding window queries against Postgres give cross-node visibility.
//
// Architecture:
//   - Increment: atomic local counter bump (zero latency on hot path)
//   - Sync loop: periodic batch upsert of pending counts to Postgres
//   - Check: Postgres sliding window query + local pending (not yet flushed)
//   - Cleanup: periodic deletion of stale rows older than 1 hour
type DistributedStore struct {
	pool   *pgxpool.Pool
	config Config
	shards [numShards]counterShard
}

// NewDistributedStore creates a new distributed store. pool may be nil, in which
// case Check will always return an error (enabling fail-closed behavior).
// The caller must call Start to begin the sync and cleanup loops.
func NewDistributedStore(pool *pgxpool.Pool, cfg Config) *DistributedStore {
	s := &DistributedStore{
		pool:   pool,
		config: cfg,
	}
	for i := range s.shards {
		s.shards[i].counters = make(map[string]*localCounter)
	}
	return s
}

// OrgScopedKey returns a rate limit key prefixed with the organization ID
// to ensure per-org rate limiting isolation.
func OrgScopedKey(orgID uuid.UUID, key string) string {
	return orgID.String() + ":" + key
}

// Start launches the sync and cleanup loops tied to the given context.
// Both loops exit when ctx is cancelled. A final flush is attempted on shutdown.
// Uses errgroup per concurrency.md (no bare go func).
func (s *DistributedStore) Start(ctx context.Context) error {
	if s.pool == nil {
		return nil // No Postgres, nothing to sync.
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.syncLoop(ctx)
	})

	g.Go(func() error {
		return s.cleanupLoop(ctx)
	})

	return g.Wait()
}

// syncLoop flushes local pending counts to Postgres on each tick of SyncInterval.
func (s *DistributedStore) syncLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush on shutdown.
			s.flushAll(context.Background())
			return nil
		case <-ticker.C:
			s.flushAll(ctx)
		}
	}
}

// cleanupLoop removes stale counter rows from Postgres every 5 minutes.
func (s *DistributedStore) cleanupLoop(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.cleanupStale(ctx)
		}
	}
}

// Increment atomically adds one to the local pending counter for the given key.
// This is the hot-path operation (no I/O, no locks on the atomic counter itself).
// The key should be org-scoped using OrgScopedKey for multi-tenant isolation.
func (s *DistributedStore) Increment(key string) {
	c := s.getOrCreateCounter(key)
	c.pending.Add(1)
}

// Check queries Postgres for the sliding window count and adds local pending.
// Returns an error if pool is nil (caller should handle fail-closed).
// Per concurrency.md: 5-second context timeout on database queries.
// The key should be org-scoped using OrgScopedKey for multi-tenant isolation.
func (s *DistributedStore) Check(ctx context.Context, key string) (int64, error) {
	if s.pool == nil {
		return 0, errors.New("distributed rate limit check failed: postgres pool is nil")
	}

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Rate limit counters use org-scoped keys, so the key itself ensures isolation.
	// We still set the org context for RLS compliance.
	orgID, orgErr := config.OrgFromContext(ctx)

	var total int64
	if orgErr == nil {
		err := config.WithOrgTx(queryCtx, s.pool, orgID, func(tx pgx.Tx) error {
			return tx.QueryRow(queryCtx,
				`SELECT COALESCE(SUM(count), 0) FROM rate_limit_counters
				 WHERE key = $1 AND window_start >= NOW() - $2::interval`,
				key, s.config.WindowSize.String(),
			).Scan(&total)
		})
		if err != nil {
			return 0, fmt.Errorf("querying rate limit counter: %w", err)
		}
	} else {
		// Fallback for contexts without org (e.g., during startup checks).
		err := s.pool.QueryRow(queryCtx,
			`SELECT COALESCE(SUM(count), 0) FROM rate_limit_counters
			 WHERE key = $1 AND window_start >= NOW() - $2::interval`,
			key, s.config.WindowSize.String(),
		).Scan(&total)
		if err != nil {
			return 0, fmt.Errorf("querying rate limit counter: %w", err)
		}
	}

	// Add local pending count not yet flushed to Postgres.
	c := s.getOrCreateCounter(key)
	total += c.pending.Load()

	return total, nil
}

// flushAll collects all pending counts and batch-upserts them to Postgres.
// If the upsert fails, pending values are added back to avoid losing counts.
func (s *DistributedStore) flushAll(ctx context.Context) {
	pending := s.collectPending()
	if len(pending) == 0 {
		return
	}

	// Extract org ID from key prefix for org-scoped inserts.
	// Keys are formatted as "{org_id}:{consumer_id}:{route}" by OrgScopedKey.
	batch := &pgx.Batch{}
	for key, count := range pending {
		orgID := extractOrgIDFromKey(key)
		batch.Queue(
			`INSERT INTO rate_limit_counters (organization_id, key, window_start, count)
			 VALUES ($1, $2, date_trunc('minute', NOW()), $3)
			 ON CONFLICT (key, window_start)
			 DO UPDATE SET count = rate_limit_counters.count + EXCLUDED.count`,
			orgID, key, count,
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	br := s.pool.SendBatch(queryCtx, batch)
	defer br.Close()

	// Check each result for errors.
	var batchErr error
	for range pending {
		if _, err := br.Exec(); err != nil {
			batchErr = err
			break
		}
	}

	if batchErr != nil {
		slog.Warn("distributed rate limit flush failed, restoring pending counts",
			slog.String("error", batchErr.Error()),
			slog.Int("keys", len(pending)),
		)
		// Restore pending counts so they aren't lost.
		for key, count := range pending {
			c := s.getOrCreateCounter(key)
			c.pending.Add(count)
		}
	}
}

// extractOrgIDFromKey extracts the organization UUID from an org-scoped key.
// Returns the default org UUID if parsing fails.
func extractOrgIDFromKey(key string) uuid.UUID {
	// Keys are formatted as "{org_uuid}:rest"
	if len(key) >= 36 {
		if id, err := uuid.Parse(key[:36]); err == nil {
			return id
		}
	}
	// Default org for backward compatibility.
	return uuid.MustParse("00000000-0000-0000-0000-000000000001")
}

// cleanupStale removes counter rows older than 1 hour from Postgres.
func (s *DistributedStore) cleanupStale(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(queryCtx,
		`DELETE FROM rate_limit_counters WHERE window_start < NOW() - INTERVAL '1 hour'`,
	)
	if err != nil {
		slog.Warn("distributed rate limit cleanup failed",
			slog.String("error", err.Error()),
		)
		return
	}

	if tag.RowsAffected() > 0 {
		slog.Debug("cleaned up stale rate limit counter rows",
			slog.Int64("rows_deleted", tag.RowsAffected()),
		)
	}
}

// collectPending atomically swaps all pending counters to zero and returns
// a map of key -> count for flushing to Postgres.
func (s *DistributedStore) collectPending() map[string]int64 {
	result := make(map[string]int64)

	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		for key, c := range sh.counters {
			val := c.pending.Swap(0)
			if val > 0 {
				result[key] = val
			}
		}
		sh.mu.RUnlock()
	}

	return result
}

// getOrCreateCounter returns the localCounter for the given key, creating it
// if it doesn't exist. Uses the same sharded map pattern as LocalLimiterStore.
func (s *DistributedStore) getOrCreateCounter(key string) *localCounter {
	idx := fnv32(key) % numShards
	sh := &s.shards[idx]

	// Fast path: read lock.
	sh.mu.RLock()
	c, ok := sh.counters[key]
	sh.mu.RUnlock()
	if ok {
		return c
	}

	// Slow path: write lock with double-check.
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if c, ok = sh.counters[key]; ok {
		return c
	}

	c = &localCounter{}
	sh.counters[key] = c
	return c
}

// getPending returns the current pending count for a key (used in tests).
func (s *DistributedStore) getPending(key string) int64 {
	idx := fnv32(key) % numShards
	sh := &s.shards[idx]

	sh.mu.RLock()
	defer sh.mu.RUnlock()

	c, ok := sh.counters[key]
	if !ok {
		return 0
	}
	return c.pending.Load()
}
