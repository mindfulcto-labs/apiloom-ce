// Package config provides configuration loading, watching, and hot-reload
// for the FlexGateway data plane.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// ConfigLoader abstracts the source of gateway configuration.
// Defined in the consumer package per Go interface conventions.
type ConfigLoader interface {
	LoadGatewayConfig(ctx context.Context) (*domain.GatewayConfig, error)
}

// LoaderOption configures optional Loader behavior.
type LoaderOption func(*Loader)

// WithOnReload registers a callback invoked after a successful config swap.
// The callback receives the previous and new configurations.
func WithOnReload(fn func(old, newCfg *domain.GatewayConfig)) LoaderOption {
	return func(l *Loader) {
		l.onReload = fn
	}
}

// Loader manages the current gateway configuration with atomic pointer swap
// for zero-drop hot reload. Get() is safe for concurrent use from any goroutine.
type Loader struct {
	current  atomic.Pointer[domain.GatewayConfig]
	source   ConfigLoader
	onReload func(old, newCfg *domain.GatewayConfig)
}

// NewLoader creates a Loader that reads configuration from the given source.
func NewLoader(source ConfigLoader, opts ...LoaderOption) *Loader {
	l := &Loader{source: source}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Get returns the current gateway configuration. Always returns non-nil
// after a successful InitialLoad call.
func (l *Loader) Get() *domain.GatewayConfig {
	return l.current.Load()
}

// InitialLoad fetches the first configuration from the source. Must be called
// once at startup before Get() is used.
func (l *Loader) InitialLoad(ctx context.Context) error {
	cfg, err := l.source.LoadGatewayConfig(ctx)
	if err != nil {
		return fmt.Errorf("initial config load: %w", err)
	}

	l.current.Store(cfg)
	slog.Info("initial config loaded",
		"version", cfg.Version,
		"routes", len(cfg.Routes),
		"backends", len(cfg.Backends),
	)

	return nil
}

// Reload fetches new configuration from the source and atomically swaps it
// if the config has changed. On error, the previous configuration is preserved.
func (l *Loader) Reload(ctx context.Context) error {
	newCfg, err := l.source.LoadGatewayConfig(ctx)
	if err != nil {
		return fmt.Errorf("reloading config: %w", err)
	}

	old := l.current.Load()

	// Skip swap if config is unchanged (compare route checksums).
	if configUnchanged(old, newCfg) {
		slog.Debug("config unchanged, skipping swap")
		return nil
	}

	l.current.Store(newCfg)

	slog.Info("config reloaded",
		"old_version", old.Version,
		"new_version", newCfg.Version,
		"old_routes", len(old.Routes),
		"new_routes", len(newCfg.Routes),
	)

	if l.onReload != nil {
		l.onReload(old, newCfg)
	}

	return nil
}

// configUnchanged compares two configs by checking route count, backend count,
// and individual route checksums. Returns true if configs are equivalent.
func configUnchanged(old, newCfg *domain.GatewayConfig) bool {
	if old == nil || newCfg == nil {
		return false
	}

	if len(old.Routes) != len(newCfg.Routes) {
		return false
	}

	if len(old.Backends) != len(newCfg.Backends) {
		return false
	}

	if old.Version != newCfg.Version {
		return false
	}

	// Build checksum set from old config.
	oldChecksums := make(map[string]struct{}, len(old.Routes))
	for _, r := range old.Routes {
		oldChecksums[r.Name+":"+r.Checksum] = struct{}{}
	}

	for _, r := range newCfg.Routes {
		if _, ok := oldChecksums[r.Name+":"+r.Checksum]; !ok {
			return false
		}
	}

	// Compare backend names (backends don't have checksums).
	oldBackends := make(map[string]struct{}, len(old.Backends))
	for _, b := range old.Backends {
		oldBackends[b.Name] = struct{}{}
	}

	for _, b := range newCfg.Backends {
		if _, ok := oldBackends[b.Name]; !ok {
			return false
		}
	}

	return true
}
