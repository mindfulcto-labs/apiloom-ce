//go:build integration

package config

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

func TestOrgContext(t *testing.T) {
	t.Run("SetOrgContext sets app.current_org within transaction", func(t *testing.T) {
		pool, _ := testutil.SetupTestDB(t, Migrations, "migrations")
		orgID := uuid.New()

		tx, err := pool.Begin(context.Background())
		require.NoError(t, err)
		defer tx.Rollback(context.Background())

		err = SetOrgContext(context.Background(), tx, orgID)
		require.NoError(t, err)

		// Verify the session variable was set.
		var currentOrg string
		err = tx.QueryRow(context.Background(), "SELECT current_setting('app.current_org')").Scan(&currentOrg)
		require.NoError(t, err)
		assert.Equal(t, orgID.String(), currentOrg)
	})

	t.Run("OrgFromContext retrieves org ID stored in Go context", func(t *testing.T) {
		orgID := uuid.New()
		ctx := ContextWithOrg(context.Background(), orgID)

		got, err := OrgFromContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, orgID, got)
	})

	t.Run("OrgFromContext returns error when not set", func(t *testing.T) {
		_, err := OrgFromContext(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization ID not found in context")
	})

	t.Run("OrgFromContext returns error for nil UUID", func(t *testing.T) {
		ctx := ContextWithOrg(context.Background(), uuid.Nil)
		_, err := OrgFromContext(ctx)
		require.Error(t, err)
	})

	t.Run("WithOrgTx wraps function in transaction with org context", func(t *testing.T) {
		pool, _ := testutil.SetupTestDB(t, Migrations, "migrations")
		orgID := uuid.New()

		var capturedOrg string
		err := WithOrgTx(context.Background(), pool, orgID, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), "SELECT current_setting('app.current_org')").Scan(&capturedOrg)
		})
		require.NoError(t, err)
		assert.Equal(t, orgID.String(), capturedOrg)
	})

	t.Run("WithOrgTx rolls back on function error", func(t *testing.T) {
		pool, _ := testutil.SetupTestDB(t, Migrations, "migrations")

		// Insert a test org first so we can use it.
		orgID := uuid.New()
		_, err := pool.Exec(context.Background(),
			"INSERT INTO organizations (id, name, slug, billing_email) VALUES ($1, $2, $3, $4)",
			orgID, "test-org", "test-org-"+orgID.String()[:8], "test@test.com")
		require.NoError(t, err)

		// WithOrgTx with an error should rollback.
		err = WithOrgTx(context.Background(), pool, orgID, func(tx pgx.Tx) error {
			_, execErr := tx.Exec(context.Background(),
				"INSERT INTO routes (name, config, organization_id) VALUES ($1, $2, $3)",
				"rollback-test", `{"path":"/rollback"}`, orgID)
			if execErr != nil {
				return execErr
			}
			return assert.AnError
		})
		require.Error(t, err)

		// The route should NOT exist since tx was rolled back.
		var count int
		err = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM routes WHERE name = 'rollback-test'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}
