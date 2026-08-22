//go:build integration

package config

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

// defaultOrgID is the well-known default organization UUID from migration 010.
var defaultOrgID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// setupStore is a per-test helper that creates a fresh store with a testcontainer.
// Each test truncates tables for isolation.
func setupStore(t *testing.T) (*ConfigStore, context.Context) {
	t.Helper()

	pool, _ := testutil.SetupTestDB(t, Migrations, "migrations")

	// Truncate tables for test isolation.
	_, err := pool.Exec(context.Background(),
		"TRUNCATE routes, backends, relationships CASCADE")
	require.NoError(t, err)

	ctx := ContextWithOrg(context.Background(), defaultOrgID)
	return NewConfigStore(pool), ctx
}

func TestSaveAndLoadRouteRoundTrip(t *testing.T) {
	store, ctx := setupStore(t)

	rc := &domain.RouteConfig{
		Name:     "test-route",
		Version:  1,
		Config:   json.RawMessage(`{"path":"/api/v1","methods":["GET","POST"]}`),
		Metadata: json.RawMessage(`{"owner":"test"}`),
	}

	saved, err := store.SaveRoute(ctx, rc)
	require.NoError(t, err)
	require.NotNil(t, saved)

	assert.NotEqual(t, uuid.Nil, saved.ID)
	assert.Equal(t, "test-route", saved.Name)
	assert.Equal(t, 1, saved.Version)
	assert.True(t, saved.IsActive)
	assert.NotEmpty(t, saved.Checksum)
	assert.Equal(t, defaultOrgID, saved.OrganizationID)
	assert.False(t, saved.CreatedAt.IsZero())
	assert.False(t, saved.UpdatedAt.IsZero())

	// Load it back by ID.
	loaded, err := store.LoadRoute(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, loaded.ID)
	assert.Equal(t, saved.Name, loaded.Name)
	assert.Equal(t, saved.Checksum, loaded.Checksum)
	assert.Equal(t, defaultOrgID, loaded.OrganizationID)
}

func TestSaveRouteDuplicateNameConflict(t *testing.T) {
	store, ctx := setupStore(t)

	rc := &domain.RouteConfig{
		Name:   "duplicate-route",
		Config: json.RawMessage(`{"path":"/dup"}`),
	}

	_, err := store.SaveRoute(ctx, rc)
	require.NoError(t, err)

	// Second save with same name should fail.
	_, err = store.SaveRoute(ctx, rc)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAlreadyExists),
		"expected ErrAlreadyExists, got: %v", err)
}

func TestLoadRoutesFiltersInactive(t *testing.T) {
	store, ctx := setupStore(t)

	// Create two routes.
	r1, err := store.SaveRoute(ctx, &domain.RouteConfig{
		Name: "active-route", Config: json.RawMessage(`{"path":"/active"}`),
	})
	require.NoError(t, err)

	_, err = store.SaveRoute(ctx, &domain.RouteConfig{
		Name: "inactive-route", Config: json.RawMessage(`{"path":"/inactive"}`),
	})
	require.NoError(t, err)

	// Soft-delete the second route.
	// We need the ID, so reload.
	allBefore, err := store.LoadRoutes(ctx)
	require.NoError(t, err)
	require.Len(t, allBefore, 2)

	var inactiveID uuid.UUID
	for _, r := range allBefore {
		if r.Name == "inactive-route" {
			inactiveID = r.ID
		}
	}

	err = store.DeleteRoute(ctx, inactiveID)
	require.NoError(t, err)

	// LoadRoutes should only return the active route.
	routes, err := store.LoadRoutes(ctx)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, r1.ID, routes[0].ID)
}

func TestDeleteRouteSoftDelete(t *testing.T) {
	store, ctx := setupStore(t)

	saved, err := store.SaveRoute(ctx, &domain.RouteConfig{
		Name: "to-delete", Config: json.RawMessage(`{"path":"/del"}`),
	})
	require.NoError(t, err)

	err = store.DeleteRoute(ctx, saved.ID)
	require.NoError(t, err)

	// Route should still exist in DB but be inactive.
	loaded, err := store.LoadRoute(ctx, saved.ID)
	require.NoError(t, err)
	assert.False(t, loaded.IsActive)

	// Deleting again should return ErrNotFound (already inactive).
	err = store.DeleteRoute(ctx, saved.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestLoadRouteNotFound(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.LoadRoute(ctx, uuid.New())
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestSaveAndLoadBackendRoundTrip(t *testing.T) {
	store, ctx := setupStore(t)

	bc := &domain.BackendConfig{
		Name:     "test-backend",
		Config:   json.RawMessage(`{"url":"http://localhost:8080","weight":100}`),
		Metadata: json.RawMessage(`{"env":"test"}`),
	}

	saved, err := store.SaveBackend(ctx, bc)
	require.NoError(t, err)
	require.NotNil(t, saved)

	assert.NotEqual(t, uuid.Nil, saved.ID)
	assert.Equal(t, "test-backend", saved.Name)
	assert.Equal(t, defaultOrgID, saved.OrganizationID)
	assert.False(t, saved.CreatedAt.IsZero())

	// Load back by ID.
	loaded, err := store.LoadBackend(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, loaded.ID)
	assert.Equal(t, saved.Name, loaded.Name)
}

func TestSaveBackendDuplicateNameConflict(t *testing.T) {
	store, ctx := setupStore(t)

	bc := &domain.BackendConfig{
		Name:   "dup-backend",
		Config: json.RawMessage(`{"url":"http://localhost"}`),
	}

	_, err := store.SaveBackend(ctx, bc)
	require.NoError(t, err)

	_, err = store.SaveBackend(ctx, bc)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAlreadyExists))
}

func TestDeleteBackend(t *testing.T) {
	store, ctx := setupStore(t)

	saved, err := store.SaveBackend(ctx, &domain.BackendConfig{
		Name: "to-delete-backend", Config: json.RawMessage(`{"url":"http://del"}`),
	})
	require.NoError(t, err)

	err = store.DeleteBackend(ctx, saved.ID)
	require.NoError(t, err)

	// Should be gone.
	_, err = store.LoadBackend(ctx, saved.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))

	// Delete again should return ErrNotFound.
	err = store.DeleteBackend(ctx, saved.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestLoadGatewayConfigAssemblesAll(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.SaveRoute(ctx, &domain.RouteConfig{
		Name: "gw-route-1", Config: json.RawMessage(`{"path":"/one"}`),
	})
	require.NoError(t, err)

	_, err = store.SaveRoute(ctx, &domain.RouteConfig{
		Name: "gw-route-2", Config: json.RawMessage(`{"path":"/two"}`),
	})
	require.NoError(t, err)

	_, err = store.SaveBackend(ctx, &domain.BackendConfig{
		Name: "gw-backend-1", Config: json.RawMessage(`{"url":"http://one"}`),
	})
	require.NoError(t, err)

	cfg, err := store.LoadGatewayConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, cfg.Routes, 2)
	assert.Len(t, cfg.Backends, 1)
	assert.False(t, cfg.LoadedAt.IsZero())
}

func TestRunMigrationsIdempotent(t *testing.T) {
	_, dsn := testutil.SetupTestDB(t, Migrations, "migrations")

	// Migrations already applied by SetupTestDB. Running again should not error.
	err := RunMigrations(context.Background(), dsn)
	require.NoError(t, err)

	// Third time for good measure.
	err = RunMigrations(context.Background(), dsn)
	require.NoError(t, err)
}
