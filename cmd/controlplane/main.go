// Package main is the entry point for the Apiloom Gateway CE control plane binary.
// It wires Postgres, runs migrations, creates the chi router with all
// REST handlers, and starts an HTTP server with graceful shutdown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/controlplane"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("apiloom-controlplane %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// run contains the application logic, separated from main for testability.
func run() error {
	// Setup structured JSON logging.
	logLevel := envOrDefault("FLEXCP_LOG_LEVEL", "info")
	setupLogger(logLevel)

	// Read required environment variables.
	dsn := os.Getenv("FLEXCP_DSN")
	if dsn == "" {
		dsn = os.Getenv("FLEXGW_DSN")
	}
	if dsn == "" {
		dsn = os.Getenv("FLEXGW_DATABASE_URL")
	}
	if dsn == "" {
		return fmt.Errorf("FLEXCP_DSN, FLEXGW_DSN, or FLEXGW_DATABASE_URL is required")
	}

	addr := envOrDefault("FLEXCP_ADDR", ":8081")

	slog.Info("apiloom-controlplane starting",
		"version", version,
		"commit", commit,
		"addr", addr,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Postgres connection pool.
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parsing DSN: %w", err)
	}
	poolCfg.MinConns = 2
	poolCfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("creating connection pool: %w", err)
	}
	defer pool.Close()

	// Verify database connectivity.
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	slog.Info("database connected")

	// Run migrations.
	if err := config.RunMigrations(ctx, dsn); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("migrations complete")

	// Create the control plane server.
	startTime := time.Now()
	srv := controlplane.NewServer(pool, version, startTime)

	// Create HTTP server.
	httpServer := &http.Server{
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	slog.Info("control plane server listening", "addr", ln.Addr().String())

	// Use errgroup for goroutine lifecycle.
	g, gCtx := errgroup.WithContext(ctx)

	// Signal handler.
	g.Go(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

		select {
		case sig := <-sigCh:
			slog.Info("received signal, initiating shutdown", "signal", sig)
			cancel()
			return nil
		case <-gCtx.Done():
			return nil
		}
	})

	// HTTP server.
	g.Go(func() error {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serving: %w", err)
		}
		return nil
	})

	// Graceful shutdown handler.
	g.Go(func() error {
		<-gCtx.Done()

		slog.Info("shutting down gracefully", "timeout", "10s")

		// Per concurrency rules: wait 3-5s for K8s endpoint removal propagation.
		time.Sleep(3 * time.Second)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("http server shutdown error", "error", err)
		}

		slog.Info("shutdown complete")
		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("errgroup: %w", err)
	}

	return nil
}

// setupLogger configures slog with JSON handler at the given level.
func setupLogger(level string) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})
	slog.SetDefault(slog.New(handler))
}

// envOrDefault returns the environment variable value or the default.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
