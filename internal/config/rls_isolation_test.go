//go:build integration

package config

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/cto-venkat/apiloom-ce/internal/testutil"
)

// appRolePool creates a connection pool using the flexgateway_app role.
// This role is subject to RLS (superusers bypass RLS even with FORCE).
// The DSN is rewritten to use flexgateway_app credentials.
func appRolePool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.User = url.UserPassword("flexgateway_app", "flexgateway_app")
	appDSN := u.String()

	poolCfg, err := pgxpool.ParseConfig(appDSN)
	require.NoError(t, err)
	poolCfg.MinConns = 2
	poolCfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	require.NoError(t, err)

	// Verify connectivity.
	require.NoError(t, pool.Ping(context.Background()))

	t.Cleanup(func() { pool.Close() })
	return pool
}

// createTestOrg inserts an organization directly and returns its UUID.
func createTestOrg(t *testing.T, pool *pgxpool.Pool, name, slug string) uuid.UUID {
	t.Helper()
	orgID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO organizations (id, name, slug, billing_email) VALUES ($1, $2, $3, $4)`,
		orgID, name, slug, name+"@test.com",
	)
	require.NoError(t, err)
	return orgID
}

func TestRLSIsolation(t *testing.T) {
	// Set up DB with superuser (runs migrations, creates flexgateway_app role).
	superPool, dsn := testutil.SetupTestDB(t, Migrations, "migrations")

	// Create two organizations using superuser (bypasses RLS for setup).
	orgA := createTestOrg(t, superPool, "Org Alpha", "org-alpha")
	orgB := createTestOrg(t, superPool, "Org Beta", "org-beta")

	// Switch to app role pool -- this pool is subject to RLS.
	pool := appRolePool(t, dsn)
	store := NewConfigStore(pool)

	ctxA := ContextWithOrg(context.Background(), orgA)
	ctxB := ContextWithOrg(context.Background(), orgB)

	// =========================================================================
	// Create identical-named entities in both orgs (should succeed -- no conflict)
	// =========================================================================

	t.Run("both orgs can create same-named route", func(t *testing.T) {
		routeA, err := store.SaveRoute(ctxA, &domain.RouteConfig{
			Name:   "payments-api",
			Config: json.RawMessage(`{"path":"/api/payments","methods":["GET","POST"]}`),
		})
		require.NoError(t, err)
		require.NotNil(t, routeA)
		assert.Equal(t, orgA, routeA.OrganizationID)

		routeB, err := store.SaveRoute(ctxB, &domain.RouteConfig{
			Name:   "payments-api",
			Config: json.RawMessage(`{"path":"/api/payments","methods":["GET"]}`),
		})
		require.NoError(t, err)
		require.NotNil(t, routeB)
		assert.Equal(t, orgB, routeB.OrganizationID)

		// Different IDs despite same name.
		assert.NotEqual(t, routeA.ID, routeB.ID)
	})

	t.Run("both orgs can create same-named backend", func(t *testing.T) {
		backendA, err := store.SaveBackend(ctxA, &domain.BackendConfig{
			Name:   "payments-backend",
			Config: json.RawMessage(`{"url":"http://payments-a:8080"}`),
		})
		require.NoError(t, err)
		assert.Equal(t, orgA, backendA.OrganizationID)

		backendB, err := store.SaveBackend(ctxB, &domain.BackendConfig{
			Name:   "payments-backend",
			Config: json.RawMessage(`{"url":"http://payments-b:8080"}`),
		})
		require.NoError(t, err)
		assert.Equal(t, orgB, backendB.OrganizationID)
	})

	t.Run("both orgs can create same-named consumer", func(t *testing.T) {
		consumerA, err := store.CreateConsumer(ctxA, "test-consumer", json.RawMessage(`{"org":"alpha"}`))
		require.NoError(t, err)
		assert.Equal(t, orgA, consumerA.OrganizationID)

		consumerB, err := store.CreateConsumer(ctxB, "test-consumer", json.RawMessage(`{"org":"beta"}`))
		require.NoError(t, err)
		assert.Equal(t, orgB, consumerB.OrganizationID)
	})

	// =========================================================================
	// Verify route isolation
	// =========================================================================

	t.Run("Org A sees only its routes", func(t *testing.T) {
		routes, err := store.LoadRoutes(ctxA)
		require.NoError(t, err)
		require.Len(t, routes, 1, "Org A should see exactly 1 route")
		assert.Equal(t, "payments-api", routes[0].Name)
		assert.Equal(t, orgA, routes[0].OrganizationID)
	})

	t.Run("Org B sees only its routes", func(t *testing.T) {
		routes, err := store.LoadRoutes(ctxB)
		require.NoError(t, err)
		require.Len(t, routes, 1, "Org B should see exactly 1 route")
		assert.Equal(t, "payments-api", routes[0].Name)
		assert.Equal(t, orgB, routes[0].OrganizationID)
	})

	// =========================================================================
	// Verify backend isolation
	// =========================================================================

	t.Run("Org A sees only its backends", func(t *testing.T) {
		backends, err := store.LoadBackends(ctxA)
		require.NoError(t, err)
		require.Len(t, backends, 1)
		assert.Equal(t, orgA, backends[0].OrganizationID)
	})

	t.Run("Org B sees only its backends", func(t *testing.T) {
		backends, err := store.LoadBackends(ctxB)
		require.NoError(t, err)
		require.Len(t, backends, 1)
		assert.Equal(t, orgB, backends[0].OrganizationID)
	})

	// =========================================================================
	// Verify consumer isolation
	// =========================================================================

	t.Run("Org A sees only its consumers", func(t *testing.T) {
		consumers, err := store.ListConsumers(ctxA)
		require.NoError(t, err)
		require.Len(t, consumers, 1)
		assert.Equal(t, orgA, consumers[0].OrganizationID)
	})

	t.Run("Org B sees only its consumers", func(t *testing.T) {
		consumers, err := store.ListConsumers(ctxB)
		require.NoError(t, err)
		require.Len(t, consumers, 1)
		assert.Equal(t, orgB, consumers[0].OrganizationID)
	})

	// =========================================================================
	// Verify API key isolation
	// =========================================================================

	t.Run("Org A API key invisible to Org B", func(t *testing.T) {
		// Get Org A's consumer to create API key.
		consumersA, err := store.ListConsumers(ctxA)
		require.NoError(t, err)
		require.Len(t, consumersA, 1)

		rawKeyA, apiKeyA, err := store.CreateAPIKey(ctxA, consumersA[0].ID, nil)
		require.NoError(t, err)
		require.NotEmpty(t, rawKeyA)
		assert.Equal(t, orgA, apiKeyA.OrganizationID)

		// Org B should not see Org A's API key by prefix.
		keysB, err := store.GetAPIKeysByPrefix(ctxB, rawKeyA[:8])
		require.NoError(t, err)
		assert.Empty(t, keysB, "Org B should NOT see Org A's API keys")
	})

	// =========================================================================
	// Verify cross-org LoadRoute returns not found
	// =========================================================================

	t.Run("Org A cannot load Org B route by ID", func(t *testing.T) {
		routesB, err := store.LoadRoutes(ctxB)
		require.NoError(t, err)
		require.Len(t, routesB, 1)

		// Try to load Org B's route using Org A context.
		_, err = store.LoadRoute(ctxA, routesB[0].ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("Org B cannot load Org A backend by ID", func(t *testing.T) {
		backendsA, err := store.LoadBackends(ctxA)
		require.NoError(t, err)
		require.Len(t, backendsA, 1)

		_, err = store.LoadBackend(ctxB, backendsA[0].ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	// =========================================================================
	// Verify cross-org delete is a no-op
	// =========================================================================

	t.Run("Org A cannot delete Org B route", func(t *testing.T) {
		routesB, err := store.LoadRoutes(ctxB)
		require.NoError(t, err)
		require.Len(t, routesB, 1)

		err = store.DeleteRoute(ctxA, routesB[0].ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)

		// Org B's route should still exist.
		routesB2, err := store.LoadRoutes(ctxB)
		require.NoError(t, err)
		require.Len(t, routesB2, 1, "Org B's route should survive cross-org delete attempt")
	})

	// =========================================================================
	// Verify NOTIFY payload includes organization_id
	// =========================================================================

	t.Run("NOTIFY payload contains organization_id", func(t *testing.T) {
		// Use a dedicated connection for LISTEN (per database.md rules).
		conn, err := pgx.Connect(context.Background(), dsn)
		require.NoError(t, err)
		defer conn.Close(context.Background())

		_, err = conn.Exec(context.Background(), "LISTEN config_changes")
		require.NoError(t, err)

		// Create a new route in Org A.
		_, err = store.SaveRoute(ctxA, &domain.RouteConfig{
			Name:   "notify-test-route",
			Config: json.RawMessage(`{"path":"/notify"}`),
		})
		require.NoError(t, err)

		// Wait for the notification.
		require.Eventually(t, func() bool {
			notifyCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			notification, waitErr := conn.WaitForNotification(notifyCtx)
			if waitErr != nil {
				return false
			}

			// Parse the JSON payload.
			var payload struct {
				Table          string `json:"table"`
				Action         string `json:"action"`
				ID             string `json:"id"`
				OrganizationID string `json:"organization_id"`
			}
			if jsonErr := json.Unmarshal([]byte(notification.Payload), &payload); jsonErr != nil {
				return false
			}

			// Verify organization_id matches Org A.
			return payload.OrganizationID == orgA.String()
		}, 5*time.Second, 100*time.Millisecond, "should receive NOTIFY with Org A's organization_id")
	})

	// =========================================================================
	// Verify LoadGatewayConfig isolation
	// =========================================================================

	t.Run("LoadGatewayConfig returns only org-scoped data", func(t *testing.T) {
		cfgA, err := store.LoadGatewayConfig(ctxA)
		require.NoError(t, err)

		cfgB, err := store.LoadGatewayConfig(ctxB)
		require.NoError(t, err)

		// Each org should see only their own routes and backends.
		for _, r := range cfgA.Routes {
			assert.Equal(t, orgA, r.OrganizationID, "Org A config should only contain Org A routes")
		}
		for _, b := range cfgA.Backends {
			assert.Equal(t, orgA, b.OrganizationID, "Org A config should only contain Org A backends")
		}
		for _, r := range cfgB.Routes {
			assert.Equal(t, orgB, r.OrganizationID, "Org B config should only contain Org B routes")
		}
		for _, b := range cfgB.Backends {
			assert.Equal(t, orgB, b.OrganizationID, "Org B config should only contain Org B backends")
		}
	})
}
