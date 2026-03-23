package budget

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresBudgetStore implements BudgetStore using PostgreSQL with
// SELECT FOR UPDATE for pessimistic reservation concurrency control.
type PostgresBudgetStore struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewPostgresBudgetStore creates a new Postgres-backed budget store.
func NewPostgresBudgetStore(pool *pgxpool.Pool) *PostgresBudgetStore {
	return &PostgresBudgetStore{
		pool:    pool,
		timeout: 5 * time.Second,
	}
}

// Reserve implements BudgetStore using pessimistic locking.
func (s *PostgresBudgetStore) Reserve(ctx context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) error {
	result, err := s.ReserveWithResult(ctx, key, estimatedTokens, estimatedCostUSD)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return ErrBudgetExceeded
	}
	return nil
}

// ReserveWithResult implements BudgetStore with detailed result.
func (s *PostgresBudgetStore) ReserveWithResult(ctx context.Context, key BudgetKey, estimatedTokens int64, estimatedCostUSD float64) (ReservationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("beginning budget tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the budget row and auto-reset period if expired.
	// Uses SELECT FOR UPDATE to prevent concurrent over-commit.
	var (
		currentTokens  int64
		currentCost    float64
		maxTokens      int64
		maxCost        float64
		enforcement    string
	)

	err = tx.QueryRow(ctx, `
		UPDATE token_budgets
		SET current_tokens = CASE
				WHEN period_start + period < NOW() THEN 0
				ELSE current_tokens
			END,
			current_cost_usd = CASE
				WHEN period_start + period < NOW() THEN 0
				ELSE current_cost_usd
			END,
			period_start = CASE
				WHEN period_start + period < NOW() THEN NOW()
				ELSE period_start
			END
		WHERE (consumer_id = $1 OR team = $2)
		RETURNING current_tokens, current_cost_usd, max_tokens, max_cost_usd, enforcement
	`, key.ConsumerID, key.Team).Scan(&currentTokens, &currentCost, &maxTokens, &maxCost, &enforcement)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("locking budget row: %w", err)
	}

	wouldExceedTokens := currentTokens+estimatedTokens > maxTokens
	wouldExceedCost := currentCost+estimatedCostUSD > maxCost

	if wouldExceedTokens || wouldExceedCost {
		if enforcement == string(HardEnforcement) {
			return ReservationResult{Allowed: false}, ErrBudgetExceeded
		}
		// Soft: allow but warn.
	}

	_, err = tx.Exec(ctx, `
		UPDATE token_budgets
		SET current_tokens = current_tokens + $1,
			current_cost_usd = current_cost_usd + $2
		WHERE (consumer_id = $3 OR team = $4)
	`, estimatedTokens, estimatedCostUSD, key.ConsumerID, key.Team)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("updating budget counters: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, fmt.Errorf("committing budget reservation: %w", err)
	}

	warning := wouldExceedTokens || wouldExceedCost
	return ReservationResult{Allowed: true, Warning: warning}, nil
}

// Refund implements BudgetStore. Credits back the delta between estimated and actual usage.
// Uses a detached context pattern: caller should create a background context with 5s timeout.
func (s *PostgresBudgetStore) Refund(ctx context.Context, key BudgetKey, estimated, actual int64, actualCostUSD float64) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tokenDelta := estimated - actual

	_, err := s.pool.Exec(ctx, `
		UPDATE token_budgets
		SET current_tokens = GREATEST(0, current_tokens - $1),
			current_cost_usd = GREATEST(0, current_cost_usd - $2)
		WHERE (consumer_id = $3 OR team = $4)
	`, tokenDelta, float64(tokenDelta)/float64(max(estimated, 1)), key.ConsumerID, key.Team)
	if err != nil {
		return fmt.Errorf("refunding budget: %w", err)
	}
	return nil
}

// CheckCeiling implements BudgetStore. Checks the per-request cost ceiling.
func (s *PostgresBudgetStore) CheckCeiling(ctx context.Context, key BudgetKey, estimatedCostUSD float64) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var ceiling *float64
	err := s.pool.QueryRow(ctx, `
		SELECT per_request_ceiling_usd
		FROM token_budgets
		WHERE (consumer_id = $1 OR team = $2)
	`, key.ConsumerID, key.Team).Scan(&ceiling)
	if err != nil {
		return fmt.Errorf("checking ceiling: %w", err)
	}

	if ceiling == nil {
		return nil
	}
	if estimatedCostUSD > *ceiling {
		return fmt.Errorf("estimated cost $%.2f exceeds ceiling $%.2f: %w",
			estimatedCostUSD, *ceiling, ErrCeilingExceeded)
	}
	return nil
}
