package config

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// orgCtxKey is the unexported context key for storing organization ID.
type orgCtxKey struct{}

// ContextWithOrg returns a new context with the organization ID stored as a value.
func ContextWithOrg(ctx context.Context, orgID uuid.UUID) context.Context {
	return context.WithValue(ctx, orgCtxKey{}, orgID)
}

// OrgFromContext retrieves the organization ID from the context.
// Returns an error if the ID is missing or nil.
func OrgFromContext(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(orgCtxKey{}).(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("organization ID not found in context")
	}
	return id, nil
}

// SetOrgContext sets the app.current_org session variable on a pgx transaction.
// MUST be called within a transaction (SET LOCAL is tx-scoped).
// Note: SET LOCAL does not support parameterized placeholders ($1), so the UUID
// is formatted into the query string. This is safe because uuid.UUID.String()
// produces a fixed-format string (8-4-4-4-12 hex digits with dashes).
func SetOrgContext(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) error {
	// Use fmt.Sprintf with single-quoted value. UUID format is injection-safe.
	query := fmt.Sprintf("SET LOCAL app.current_org = '%s'", orgID.String())
	_, err := tx.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("setting org context: %w", err)
	}
	return nil
}

// WithOrgTx begins a transaction, sets the org context, calls fn, and commits.
// On error from fn, the transaction is rolled back.
func WithOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning org tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := SetOrgContext(ctx, tx, orgID); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
