package gateway

import (
	"context"
	"fmt"
	"log/slog"
)

// residencyRegionKey is the typed context key for the consumer's data residency region.
type residencyRegionKey struct{}

// WithResidencyRegion attaches a data residency region to the context.
// This is typically set during authentication when the consumer's region is known.
func WithResidencyRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, residencyRegionKey{}, region)
}

// ResidencyRegionFromCtx extracts the data residency region from the context.
// Returns empty string if not set.
func ResidencyRegionFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(residencyRegionKey{}).(string)
	return v
}

// ResidencyConfig defines region-to-backend URL mappings for a route.
// Stored in route ExtraConfig under key "flexgateway/residency".
type ResidencyConfig struct {
	Regions map[string]string `json:"regions"`
	Default string            `json:"default"`
}

// ResidencyBackend is the resolved backend for a residency-routed request.
type ResidencyBackend struct {
	URL    string
	Region string
}

// ResidencyRouter selects backends based on the consumer's data residency region.
type ResidencyRouter struct {
	logger *slog.Logger
}

// NewResidencyRouter creates a ResidencyRouter with the given logger.
// If logger is nil, slog.Default() is used.
func NewResidencyRouter(logger *slog.Logger) *ResidencyRouter {
	if logger == nil {
		logger = slog.Default()
	}
	return &ResidencyRouter{logger: logger}
}

// RouteByResidency resolves a backend URL based on the consumer's data residency
// region from context and the route's residency configuration.
// If the consumer has no region or the region is not in config, falls back to default.
func (rr *ResidencyRouter) RouteByResidency(ctx context.Context, cfg ResidencyConfig) (*ResidencyBackend, error) {
	if cfg.Default == "" && len(cfg.Regions) == 0 {
		return nil, fmt.Errorf("routing by residency: no backends configured")
	}

	region := ResidencyRegionFromCtx(ctx)

	// Look up region-specific backend.
	if region != "" {
		if backendURL, ok := cfg.Regions[region]; ok {
			rr.logger.LogAttrs(ctx, slog.LevelInfo, "residency routing",
				slog.String("region", region),
				slog.String("backend", backendURL),
			)
			return &ResidencyBackend{URL: backendURL, Region: region}, nil
		}
	}

	// Fall back to default.
	if cfg.Default == "" {
		return nil, fmt.Errorf("routing by residency: no default backend for region %q", region)
	}

	rr.logger.LogAttrs(ctx, slog.LevelInfo, "residency routing fallback",
		slog.String("region", region),
		slog.String("backend", cfg.Default),
	)
	return &ResidencyBackend{URL: cfg.Default, Region: "default"}, nil
}
