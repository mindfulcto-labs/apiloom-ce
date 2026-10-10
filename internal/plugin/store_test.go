//go:build integration

package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

// newTestOrgContext creates a fresh organisation and returns a context scoped
// to it. The default organisation holds demo seed data from the migrations,
// so tests that count or name plugins need their own organisation.
func newTestOrgContext(t *testing.T, pool *pgxpool.Pool) context.Context {
	t.Helper()
	orgID := uuid.New()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO organizations (id, name, slug, billing_email) VALUES ($1, $2, $3, $4)",
		orgID, "test-org", "test-org-"+orgID.String()[:8], "test@example.com")
	require.NoError(t, err)
	return config.ContextWithOrg(context.Background(), orgID)
}

func TestPostgresStore_Create(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	tests := []struct {
		name    string
		rec     *PluginRecord
		wantErr error
	}{
		{
			name: "create native plugin",
			rec: &PluginRecord{
				Name:       "rate-limit",
				PluginType: "native",
				Config:     json.RawMessage(`{"rps": 100}`),
				Target:     "global",
				Ordering:   10,
			},
		},
		{
			name: "create grpc plugin",
			rec: &PluginRecord{
				Name:        "custom-auth",
				PluginType:  "grpc",
				Config:      json.RawMessage(`{}`),
				Target:      "route",
				GRPCAddress: strPtr("localhost:50051"),
				Ordering:    20,
			},
		},
		{
			name: "duplicate name returns conflict",
			rec: &PluginRecord{
				Name:       "rate-limit",
				PluginType: "native",
				Config:     json.RawMessage(`{}`),
				Target:     "global",
			},
			wantErr: domain.ErrAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.Create(ctx, tt.rec)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.Equal(t, tt.rec.Name, result.Name)
			assert.Equal(t, tt.rec.PluginType, result.PluginType)
			assert.Equal(t, tt.rec.Target, result.Target)
			assert.Equal(t, tt.rec.Ordering, result.Ordering)
			assert.False(t, result.IsEnabled)
			assert.False(t, result.CreatedAt.IsZero())
			assert.False(t, result.UpdatedAt.IsZero())
		})
	}
}

func TestPostgresStore_Get(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	created, err := store.Create(ctx, &PluginRecord{
		Name:       "test-get",
		PluginType: "native",
		Config:     json.RawMessage(`{"key": "value"}`),
		Target:     "global",
		Ordering:   5,
	})
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		got, err := store.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "test-get", got.Name)
		assert.Equal(t, "native", got.PluginType)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := store.Get(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPostgresStore_List(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	// Create a few plugins with slight delays for ordering.
	for i := 0; i < 3; i++ {
		rec := &PluginRecord{
			Name:       "list-plugin-" + string(rune('a'+i)),
			PluginType: "native",
			Config:     json.RawMessage(`{}`),
			Target:     "global",
			Ordering:   i,
		}
		_, err := store.Create(ctx, rec)
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond) // ensure different created_at
	}

	// Enable one.
	all, err := store.List(ctx, ListPluginParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, all.Data, 3)
	_, err = store.UpdateEnabled(ctx, all.Data[1].ID, true)
	require.NoError(t, err)

	t.Run("list all", func(t *testing.T) {
		result, err := store.List(ctx, ListPluginParams{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, result.Data, 3)
		assert.False(t, result.HasMore)
	})

	t.Run("filter by enabled", func(t *testing.T) {
		enabled := true
		result, err := store.List(ctx, ListPluginParams{Limit: 10, IsEnabled: &enabled})
		require.NoError(t, err)
		assert.Len(t, result.Data, 1)
		assert.True(t, result.Data[0].IsEnabled)
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := store.List(ctx, ListPluginParams{Limit: 10, PluginType: "grpc"})
		require.NoError(t, err)
		assert.Len(t, result.Data, 0)
	})

	t.Run("pagination", func(t *testing.T) {
		page1, err := store.List(ctx, ListPluginParams{Limit: 2})
		require.NoError(t, err)
		assert.Len(t, page1.Data, 2)
		assert.True(t, page1.HasMore)
		assert.NotEmpty(t, page1.NextCursor)

		page2, err := store.List(ctx, ListPluginParams{Limit: 2, Cursor: page1.NextCursor})
		require.NoError(t, err)
		assert.Len(t, page2.Data, 1)
		assert.False(t, page2.HasMore)
	})
}

func TestPostgresStore_UpdateConfig(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	created, err := store.Create(ctx, &PluginRecord{
		Name:       "config-update",
		PluginType: "native",
		Config:     json.RawMessage(`{"old": true}`),
		Target:     "global",
	})
	require.NoError(t, err)

	updated, err := store.UpdateConfig(ctx, created.ID, json.RawMessage(`{"new": true}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"new": true}`, string(updated.Config))
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt))
}

func TestPostgresStore_UpdateEnabled(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	created, err := store.Create(ctx, &PluginRecord{
		Name:       "enable-test",
		PluginType: "native",
		Config:     json.RawMessage(`{}`),
		Target:     "global",
	})
	require.NoError(t, err)
	assert.False(t, created.IsEnabled)

	enabled, err := store.UpdateEnabled(ctx, created.ID, true)
	require.NoError(t, err)
	assert.True(t, enabled.IsEnabled)

	disabled, err := store.UpdateEnabled(ctx, created.ID, false)
	require.NoError(t, err)
	assert.False(t, disabled.IsEnabled)
}

func TestPostgresStore_UpdateOrdering(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	created, err := store.Create(ctx, &PluginRecord{
		Name:       "ordering-test",
		PluginType: "native",
		Config:     json.RawMessage(`{}`),
		Target:     "global",
		Ordering:   0,
	})
	require.NoError(t, err)

	updated, err := store.UpdateOrdering(ctx, created.ID, 42)
	require.NoError(t, err)
	assert.Equal(t, 42, updated.Ordering)
}

func TestPostgresStore_Delete(t *testing.T) {
	pool, _ := testutil.SetupTestDB(t, config.Migrations, "migrations")
	store := NewPostgresStore(pool)
	ctx := newTestOrgContext(t, pool)

	created, err := store.Create(ctx, &PluginRecord{
		Name:       "delete-test",
		PluginType: "native",
		Config:     json.RawMessage(`{}`),
		Target:     "global",
	})
	require.NoError(t, err)

	err = store.Delete(ctx, created.ID)
	require.NoError(t, err)

	_, err = store.Get(ctx, created.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Delete non-existent.
	err = store.Delete(ctx, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func strPtr(s string) *string {
	return &s
}
