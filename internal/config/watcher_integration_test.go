//go:build integration

package config

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/cto-venkat/apiloom-ce/internal/testutil"
)

var (
	testPool *pgxpool.Pool
	testDSN  string
)

func TestMain(m *testing.M) {
	// Integration tests share a single testcontainer.
	// SetupTestDB is called from the first test; we just run tests.
	os.Exit(m.Run())
}

func TestWatcherIntegration_NotifyTriggersReload(t *testing.T) {
	pool, dsn := testutil.SetupTestDB(t, Migrations, "migrations")
	store := NewConfigStore(pool)

	// Use the default org context for all operations.
	ctx := ContextWithOrg(context.Background(), defaultOrgID)

	loader := NewLoader(store)
	require.NoError(t, loader.InitialLoad(ctx))
	initialRoutes := len(loader.Get().Routes)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	watcher := NewWatcher(dsn, loader, WithLogger(logger), WithDebounceInterval(50*time.Millisecond))

	// Start watcher in background.
	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- watcher.Run(watchCtx)
	}()

	// Give watcher time to connect and LISTEN.
	time.Sleep(500 * time.Millisecond)

	// Insert a route -- the NOTIFY trigger should fire.
	_, err := store.SaveRoute(ctx, &domain.RouteConfig{
		Name:   "test-watcher-route",
		Config: json.RawMessage(`{"path":"/watcher-test"}`),
	})
	require.NoError(t, err)

	// Loader should eventually reflect the new route.
	require.Eventually(t, func() bool {
		return len(loader.Get().Routes) > initialRoutes
	}, 5*time.Second, 100*time.Millisecond, "watcher should reload config after NOTIFY")

	// Verify the new route is present.
	found := false
	for _, r := range loader.Get().Routes {
		if r.Name == "test-watcher-route" {
			found = true
			break
		}
	}
	require.True(t, found, "new route should be visible after reload")

	// Clean shutdown.
	cancel()

	select {
	case err := <-watcherDone:
		require.NoError(t, err, "watcher should return nil on clean shutdown")
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not shut down within timeout")
	}
}
