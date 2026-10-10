// Package main is the entry point for the Apiloom Gateway CE data plane binary.
// It wires config store, loader, watcher, server, and health endpoints
// into a single process with graceful shutdown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/gateway"
	"github.com/mindfulcto-labs/apiloom-ce/internal/observability"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Printf("apiloom-gateway %s (commit: %s, built: %s)\n", version, commit, date)
			os.Exit(0)
		case "healthcheck":
			resp, err := http.Get("http://localhost:8080/healthz")
			if err != nil || resp.StatusCode != 200 {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// run contains the actual application logic, separated from main for testability.
func run() error {
	// Parse environment variables.
	cfg := parseEnvConfig()

	// Setup structured logging.
	setupLogger(cfg.logLevel)

	slog.Info("apiloom-gateway starting",
		"version", version,
		"commit", commit,
		"listen_addr", cfg.listenAddr,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Postgres connection pool.
	poolCfg, err := pgxpool.ParseConfig(cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("parsing database URL: %w", err)
	}
	poolCfg.MinConns = parsePoolSizeOrDefault("FLEXGW_DB_MIN_CONNS", 5)
	poolCfg.MaxConns = parsePoolSizeOrDefault("FLEXGW_DB_MAX_CONNS", 20)

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
	if err := config.RunMigrations(ctx, cfg.databaseURL); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("migrations complete")

	// Create config store, loader, and watcher.
	var reloadCallback func(old, newCfg *domain.GatewayConfig)
	store := config.NewConfigStore(pool)
	loader := config.NewLoader(store, config.WithOnReload(func(old, newCfg *domain.GatewayConfig) {
		if reloadCallback != nil {
			reloadCallback(old, newCfg)
		}
	}))
	watcher := config.NewWatcher(cfg.databaseURL, loader,
		config.WithLogger(slog.Default()),
	)

	// Initial config load. On first-time empty DB, log warning but continue.
	if err := loader.InitialLoad(ctx); err != nil {
		slog.Warn("initial config load failed, continuing with empty config", "error", err)
	}

	// Initialize OTel tracing.
	tracingShutdown, err := observability.InitTracer(ctx, observability.TracingConfig{
		ServiceName:    "apiloom-gateway",
		OTLPEndpoint:   cfg.otlpEndpoint,
		SampleRate:     cfg.otelSampleRate,
		ServiceVersion: version,
	})
	if err != nil {
		return fmt.Errorf("initializing tracer: %w", err)
	}

	// Create Prometheus registry and metrics.
	promReg := prometheus.NewRegistry()
	gwMetrics := observability.NewMetrics(promReg)

	slog.Info("observability initialized",
		"otlp_endpoint", cfg.otlpEndpoint,
		"sample_rate", cfg.otelSampleRate,
		"metrics_path", cfg.metricsPath,
	)

	// Create block chain (CE: core blocks only, no traffic observation).
	chain := gateway.NewBlockChain()

	// Create health handler.
	healthHandler := gateway.NewHealthHandler(loader, pool)

	// Build dynamic gateway handler that rebuilds on config reload.
	dynamicHandler, err := gateway.NewDynamicHandler(loader, chain, slog.Default())
	if err != nil {
		return fmt.Errorf("building gateway handler: %w", err)
	}

	// Wire config reload to handler rebuild.
	reloadCallback = func(_, _ *domain.GatewayConfig) {
		if err := dynamicHandler.Rebuild(); err != nil {
			slog.Error("failed to rebuild handler on config reload", "error", err)
		}
	}

	// Wrap gateway handler with observability middleware.
	var wrappedHandler http.Handler = dynamicHandler
	wrappedHandler = gwMetrics.Middleware(wrappedHandler)
	wrappedHandler = observability.TracingMiddleware(wrappedHandler)
	wrappedHandler = observability.LoggingMiddleware(slog.Default(), observability.LoggingConfig{
		Level: slog.LevelDebug,
	})(wrappedHandler)

	// Combine health endpoints, metrics, and gateway handler on a single mux.
	combined := buildCombinedHandler(healthHandler, wrappedHandler, cfg.metricsPath, observability.MetricsHandler(promReg))

	// Create the HTTP server.
	httpServer := &http.Server{
		Handler:      combined,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
		IdleTimeout:  cfg.idleTimeout,
	}

	ln, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", cfg.listenAddr, err)
	}

	slog.Info("gateway server listening", "addr", ln.Addr().String())

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

	// Config watcher.
	g.Go(func() error {
		return watcher.Run(gCtx)
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

		slog.Info("shutting down gracefully", "timeout", cfg.shutdownTimeout)

		// Per concurrency rules: wait 3-5s for K8s endpoint removal propagation.
		time.Sleep(3 * time.Second)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("http server shutdown error", "error", err)
		}

		// Flush OTel spans and metrics.
		otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer otelCancel()
		if err := tracingShutdown(otelCtx); err != nil {
			slog.Error("otel tracing shutdown error", "error", err)
		}

		slog.Info("shutdown complete")
		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("errgroup: %w", err)
	}

	return nil
}

// buildCombinedHandler returns an http.Handler that routes health endpoints,
// the metrics endpoint, and gateway traffic.
func buildCombinedHandler(health *gateway.HealthHandler, gw http.Handler, metricsPath string, metricsHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			health.HandleLiveness(w, r)
		case r.URL.Path == "/readyz":
			health.HandleReadiness(w, r)
		case strings.HasPrefix(r.URL.Path, "/healthz/"):
			health.HandleDeep(w, r)
		case r.URL.Path == metricsPath:
			metricsHandler.ServeHTTP(w, r)
		default:
			gw.ServeHTTP(w, r)
		}
	})
}

// envConfig holds parsed environment configuration.
type envConfig struct {
	databaseURL     string
	listenAddr      string
	logLevel        string
	shutdownTimeout time.Duration
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	otlpEndpoint    string
	otelSampleRate  float64
	metricsPath     string
}

// parseEnvConfig reads configuration from environment variables with defaults.
func parseEnvConfig() envConfig {
	cfg := envConfig{
		databaseURL:     os.Getenv("FLEXGW_DATABASE_URL"),
		listenAddr:      envOrDefault("FLEXGW_LISTEN_ADDR", ":8080"),
		logLevel:        envOrDefault("FLEXGW_LOG_LEVEL", "info"),
		shutdownTimeout: parseDurationOrDefault("FLEXGW_SHUTDOWN_TIMEOUT", 30*time.Second),
		readTimeout:     parseDurationOrDefault("FLEXGW_READ_TIMEOUT", 30*time.Second),
		writeTimeout:    parseDurationOrDefault("FLEXGW_WRITE_TIMEOUT", 60*time.Second),
		idleTimeout:     parseDurationOrDefault("FLEXGW_IDLE_TIMEOUT", 120*time.Second),
		otlpEndpoint:    os.Getenv("FLEXGW_OTLP_ENDPOINT"),
		otelSampleRate:  parseFloatOrDefault("FLEXGW_OTEL_SAMPLE_RATE", 1.0),
		metricsPath:     envOrDefault("FLEXGW_METRICS_PATH", "/metrics"),
	}

	if cfg.databaseURL == "" {
		slog.Error("FLEXGW_DATABASE_URL is required")
		os.Exit(1)
	}

	return cfg
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

// parseFloatOrDefault parses a float64 from env var or returns the default.
func parseFloatOrDefault(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("invalid float, using default", "key", key, "value", v, "default", defaultVal)
		return defaultVal
	}
	return f
}

// parseDurationOrDefault parses a duration from env var or returns the default.
func parseDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("invalid duration, using default", "key", key, "value", v, "default", defaultVal)
		return defaultVal
	}
	return d
}

// parseIntOrDefault reads an int from env var or returns the default.
func parseIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid int, using default", "key", key, "value", v, "default", defaultVal)
		return defaultVal
	}
	return n
}

// maxPoolSize is an upper bound on database pool sizes read from the environment.
const maxPoolSize = 1000

// parsePoolSizeOrDefault reads a pool size from the environment. Values below 0
// or above maxPoolSize are rejected and the default is used instead.
func parsePoolSizeOrDefault(key string, defaultVal int32) int32 {
	n := parseIntOrDefault(key, int(defaultVal))
	if n < 0 || n > maxPoolSize {
		slog.Warn("pool size out of range, using default", "key", key, "value", n, "default", defaultVal)
		return defaultVal
	}
	return int32(n)
}
