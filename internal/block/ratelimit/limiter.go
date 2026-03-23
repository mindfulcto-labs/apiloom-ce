package ratelimit

import (
	"hash/fnv"
	"sync"

	"golang.org/x/time/rate"
)

const numShards = 256

// shard holds a subset of rate limiters protected by a read-write mutex.
type shard struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
}

// LocalLimiterStore is a sharded in-memory store for per-key token bucket rate
// limiters. It uses 256 shards with per-shard RWMutex to avoid contention,
// following concurrency.md (no sync.Map for rate limit state).
type LocalLimiterStore struct {
	shards       [numShards]shard
	defaultRate  rate.Limit
	defaultBurst int
}

// NewLocalLimiterStore creates a new sharded limiter store with the given
// default rate (requests per second) and burst size.
func NewLocalLimiterStore(r float64, burst int) *LocalLimiterStore {
	s := &LocalLimiterStore{
		defaultRate:  rate.Limit(r),
		defaultBurst: burst,
	}
	for i := range s.shards {
		s.shards[i].limiters = make(map[string]*rate.Limiter)
	}
	return s
}

// Get returns the rate limiter for the given key, creating one with default
// rate/burst if it does not exist.
func (s *LocalLimiterStore) Get(key string) *rate.Limiter {
	idx := fnv32(key) % numShards
	sh := &s.shards[idx]

	// Fast path: read lock.
	sh.mu.RLock()
	lim, ok := sh.limiters[key]
	sh.mu.RUnlock()
	if ok {
		return lim
	}

	// Slow path: write lock with double-check.
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if lim, ok = sh.limiters[key]; ok {
		return lim
	}

	lim = rate.NewLimiter(s.defaultRate, s.defaultBurst)
	sh.limiters[key] = lim
	return lim
}

// GetWithOverride returns the rate limiter for the given key, creating one with
// the specified rate/burst if it does not exist. If the limiter already exists
// with a different rate or burst, it updates the limiter in place.
func (s *LocalLimiterStore) GetWithOverride(key string, r float64, burst int) *rate.Limiter {
	idx := fnv32(key) % numShards
	sh := &s.shards[idx]
	rl := rate.Limit(r)

	// Fast path: read lock check.
	sh.mu.RLock()
	lim, ok := sh.limiters[key]
	sh.mu.RUnlock()
	if ok {
		// Update rate/burst if changed.
		if lim.Limit() != rl {
			lim.SetLimit(rl)
		}
		if lim.Burst() != burst {
			lim.SetBurst(burst)
		}
		return lim
	}

	// Slow path: write lock with double-check.
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if lim, ok = sh.limiters[key]; ok {
		if lim.Limit() != rl {
			lim.SetLimit(rl)
		}
		if lim.Burst() != burst {
			lim.SetBurst(burst)
		}
		return lim
	}

	lim = rate.NewLimiter(rl, burst)
	sh.limiters[key] = lim
	return lim
}

// Len returns the total number of entries across all shards.
func (s *LocalLimiterStore) Len() int {
	total := 0
	for i := range s.shards {
		s.shards[i].mu.RLock()
		total += len(s.shards[i].limiters)
		s.shards[i].mu.RUnlock()
	}
	return total
}

// fnv32 computes an FNV-1a hash of the key for shard selection.
func fnv32(key string) uint32 {
	h := fnv.New32a()
	// fnv.Write never returns an error.
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}
