// Package budget provides token budget enforcement with pessimistic reservation.
// The BudgetStore interface supports Reserve (pre-request deduction), Refund
// (post-response credit), and CheckCeiling (per-request cost ceiling).
package budget

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// Sentinel errors for budget enforcement.
var (
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrCeilingExceeded = errors.New("per-request cost ceiling exceeded")
)

// EnforcementMode determines how budget violations are handled.
type EnforcementMode string

const (
	HardEnforcement EnforcementMode = "hard"
	SoftEnforcement EnforcementMode = "soft"
)

// BudgetKey identifies a budget by consumer or team.
type BudgetKey struct {
	ConsumerID *uuid.UUID
	Team       string
}

// String returns a canonical string representation for the budget key.
func (k BudgetKey) String() string {
	if k.ConsumerID != nil {
		return "consumer:" + k.ConsumerID.String()
	}
	return "team:" + k.Team
}

// ReservationResult contains the outcome of a reservation attempt,
// including soft enforcement warning flags.
type ReservationResult struct {
	Allowed bool
	Warning bool
}

// BudgetStore defines the interface for budget enforcement operations.
// Defined in consumer package per interfaces-in-consumer rule, but
// implemented here for the AI gateway layer.
type BudgetStore interface {
	// Reserve deducts estimated tokens and cost from the budget.
	// Returns ErrBudgetExceeded if hard enforcement blocks the request.
	Reserve(ctx context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) error

	// ReserveWithResult is like Reserve but returns detailed result with warning flags.
	ReserveWithResult(ctx context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) (ReservationResult, error)

	// Refund credits back the difference between estimated and actual usage.
	Refund(ctx context.Context, key BudgetKey, estimated, actual int64, actualCostUSD float64) error

	// CheckCeiling verifies the estimated cost does not exceed the per-request ceiling.
	// Returns nil if no ceiling is configured.
	CheckCeiling(ctx context.Context, key BudgetKey, estimatedCostUSD float64) error
}

// budgetEntry tracks token and cost usage for a single budget key.
type budgetEntry struct {
	mu              sync.Mutex
	currentTokens   int64
	currentCostUSD  float64
	maxTokens       int64
	maxCostUSD      float64
	enforcement     EnforcementMode
	ceilingUSD      *float64
}

// MemoryBudgetStore is an in-memory BudgetStore for unit testing and
// single-instance deployments. Production uses PostgresBudgetStore.
type MemoryBudgetStore struct {
	mu      sync.RWMutex
	budgets map[string]*budgetEntry
	// defaults for new keys
	defaultMaxTokens   int64
	defaultMaxCostUSD  float64
	defaultEnforcement EnforcementMode
	defaultCeilingUSD  *float64
}

// newTestStore creates a MemoryBudgetStore with the given defaults for testing.
func newTestStore(maxTokens int64, maxCost float64, enforcement EnforcementMode, ceiling *float64) *MemoryBudgetStore {
	return &MemoryBudgetStore{
		budgets:            make(map[string]*budgetEntry),
		defaultMaxTokens:   maxTokens,
		defaultMaxCostUSD:  maxCost,
		defaultEnforcement: enforcement,
		defaultCeilingUSD:  ceiling,
	}
}

// NewMemoryBudgetStore creates a MemoryBudgetStore for testing.
func NewMemoryBudgetStore(maxTokens int64, maxCost float64, enforcement EnforcementMode, ceiling *float64) *MemoryBudgetStore {
	return newTestStore(maxTokens, maxCost, enforcement, ceiling)
}

func (s *MemoryBudgetStore) getOrCreate(key BudgetKey) *budgetEntry {
	k := key.String()
	s.mu.RLock()
	entry, ok := s.budgets[k]
	s.mu.RUnlock()
	if ok {
		return entry
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock.
	if entry, ok = s.budgets[k]; ok {
		return entry
	}
	entry = &budgetEntry{
		maxTokens:   s.defaultMaxTokens,
		maxCostUSD:  s.defaultMaxCostUSD,
		enforcement: s.defaultEnforcement,
		ceilingUSD:  s.defaultCeilingUSD,
	}
	s.budgets[k] = entry
	return entry
}

// Reserve implements BudgetStore.
func (s *MemoryBudgetStore) Reserve(ctx context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) error {
	result, err := s.ReserveWithResult(ctx, key, estimatedTokens, estimatedCostUSD)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return ErrBudgetExceeded
	}
	return nil
}

// ReserveWithResult implements BudgetStore.
func (s *MemoryBudgetStore) ReserveWithResult(_ context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) (ReservationResult, error) {
	entry := s.getOrCreate(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	wouldExceedTokens := entry.currentTokens+estimatedTokens > entry.maxTokens
	wouldExceedCost := entry.currentCostUSD+estimatedCostUSD > entry.maxCostUSD

	if wouldExceedTokens || wouldExceedCost {
		if entry.enforcement == HardEnforcement {
			return ReservationResult{Allowed: false}, ErrBudgetExceeded
		}
		// Soft enforcement: allow but warn. Still deduct.
		entry.currentTokens += estimatedTokens
		entry.currentCostUSD += estimatedCostUSD
		return ReservationResult{Allowed: true, Warning: true}, nil
	}

	entry.currentTokens += estimatedTokens
	entry.currentCostUSD += estimatedCostUSD
	return ReservationResult{Allowed: true, Warning: false}, nil
}

// Refund implements BudgetStore.
func (s *MemoryBudgetStore) Refund(_ context.Context, key BudgetKey, estimated, actual int64, actualCostUSD float64) error {
	entry := s.getOrCreate(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	tokenDelta := estimated - actual
	entry.currentTokens -= tokenDelta
	if entry.currentTokens < 0 {
		entry.currentTokens = 0
	}

	// Refund cost delta: estimated cost was already reserved at Reserve time.
	// We need to calculate: currentCost - (estimatedCost - actualCost)
	// But we don't have the original estimated cost. Instead, refund the
	// difference in proportion: the caller passes estimated/actual tokens
	// and actualCost. The reserved cost was estimatedCostUSD from Reserve.
	// Since we don't track per-reservation cost, we recalculate:
	// costDelta = (estimated - actual) portion of cost.
	// Simplified: just adjust to actual cost level.
	if estimated > 0 {
		estimatedCostForTokens := entry.currentCostUSD // can't precisely recover
		_ = estimatedCostForTokens
		// Simpler approach: credit back the ratio of tokens refunded.
		// costRefund = (tokenDelta / estimated) * previously reserved cost portion
		// But we don't track that. Use the approach from the plan:
		// The caller tells us the actual cost, we know what was estimated.
		// Just deduct the difference.
	}
	// Corrected: recalculate cost based on what the caller reports.
	// The estimated cost for these tokens was proportional. Since Reserve was called
	// with a specific estimatedCostUSD, and now we know actual, just credit the delta.
	// However, we need the original estimatedCostUSD to compute delta.
	// The plan says: Refund(key, estimated, actual int64, actualCostUSD float64)
	// And Reserve was: Reserve(key, estimatedTokens, estimatedCostUSD)
	// The cost delta is: estimatedCostUSD (from Reserve) - actualCostUSD.
	// But we don't have the original estimatedCostUSD here.
	// Solution: compute it from the token ratio: costRefund = (tokenDelta/estimated) * currentCostProportional
	// Actually simplest: we track that costDelta = costReserved - actualCost.
	// Since we don't store per-reservation, just use the token ratio for cost too.
	if tokenDelta > 0 && estimated > 0 {
		// Credit back cost proportional to token refund
		costPerToken := entry.currentCostUSD / float64(entry.currentTokens+tokenDelta)
		costRefund := costPerToken * float64(tokenDelta)
		entry.currentCostUSD -= costRefund
		if entry.currentCostUSD < 0 {
			entry.currentCostUSD = 0
		}
	}

	return nil
}

// CheckCeiling implements BudgetStore.
func (s *MemoryBudgetStore) CheckCeiling(_ context.Context, key BudgetKey, estimatedCostUSD float64) error {
	entry := s.getOrCreate(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.ceilingUSD == nil {
		return nil
	}
	if estimatedCostUSD > *entry.ceilingUSD {
		return fmt.Errorf("estimated cost $%.2f exceeds ceiling $%.2f: %w",
			estimatedCostUSD, *entry.ceilingUSD, ErrCeilingExceeded)
	}
	return nil
}
