//go:build integration

// Package testutil provides shared test infrastructure for FlexGateway integration tests.
package testutil

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SetupTestDB starts a Postgres 16-alpine testcontainer, applies migrations from
// the provided embedded filesystem, and returns a connection pool and DSN.
// The migrationsFS should contain migration files at the given migrationsPath
// (e.g., config.Migrations with path "migrations").
// The container and pool are cleaned up automatically via t.Cleanup.
func SetupTestDB(t testing.TB, migrationsFS fs.FS, migrationsPath string) (*pgxpool.Pool, string) {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("flexgateway_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("terminating postgres container: %v", err)
		}
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	// Verify we can actually connect before running migrations.
	verifyPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("creating verify pool: %v", err)
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := verifyPool.Ping(pingCtx); err != nil {
		t.Fatalf("pinging database before migrations: %v", err)
	}
	verifyPool.Close()

	// Apply migrations.
	src, err := iofs.New(migrationsFS, migrationsPath)
	if err != nil {
		t.Fatalf("creating migration source: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("creating migrator: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("running migrations: %v", err)
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		t.Fatalf("closing migration source: %v", srcErr)
	}
	if dbErr != nil {
		t.Fatalf("closing migration db: %v", dbErr)
	}

	// Create connection pool.
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing pool config: %v", err)
	}
	poolCfg.MinConns = 2
	poolCfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("creating pool: %v", err)
	}

	// Verify connectivity.
	finalPingCtx, finalCancel := context.WithTimeout(ctx, 5*time.Second)
	defer finalCancel()
	if err := pool.Ping(finalPingCtx); err != nil {
		t.Fatalf("pinging database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool, dsn
}
