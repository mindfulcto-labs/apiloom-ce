package gateway

import (
	"context"
	"crypto/rand"
	"log/slog"
	"math/big"

	"github.com/google/uuid"
	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// canaryContextKey is the unexported key for storing the active version in the context.
type canaryContextKey struct{}

// CanaryVersionFromContext retrieves the selected canary version config from the context.
// Returns nil if no canary routing decision was made.
func CanaryVersionFromContext(ctx context.Context) *domain.APIVersion {
	v, _ := ctx.Value(canaryContextKey{}).(*domain.APIVersion)
	return v
}

// CanaryVersionGetter retrieves an API version by ID. Used by CanaryBlock
// to load the selected version's config.
type CanaryVersionGetter interface {
	GetAPIVersion(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error)
}

// CanaryConfigGetter retrieves the active canary config for a route.
type CanaryConfigGetter interface {
	GetCanary(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error)
}

// CanaryBlock implements weight-based canary routing as a gateway Block.
// It reads canary config for the route and selects either the primary
// or canary version based on a cryptographically random number.
type CanaryBlock struct {
	configGetter  CanaryConfigGetter
	versionGetter CanaryVersionGetter
	logger        *slog.Logger
}

// NewCanaryBlock creates a CanaryBlock. Returns a passthrough block if
// configGetter or versionGetter is nil.
func NewCanaryBlock(configGetter CanaryConfigGetter, versionGetter CanaryVersionGetter, logger *slog.Logger) *CanaryBlock {
	if logger == nil {
		logger = slog.Default()
	}
	return &CanaryBlock{
		configGetter:  configGetter,
		versionGetter: versionGetter,
		logger:        logger,
	}
}

// Name returns the block identifier.
func (b *CanaryBlock) Name() string {
	return "canary-routing"
}

// Middleware returns a Lura-compatible proxy middleware that performs
// weight-based canary routing. If no canary config exists for the route,
// the request passes through unchanged.
func (b *CanaryBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) == 0 {
			return proxy.NoopProxy
		}
		backend := next[0]

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			// No config getter means passthrough.
			if b.configGetter == nil || b.versionGetter == nil {
				return backend(ctx, req)
			}

			// Extract route ID from request context or URL.
			routeID := routeIDFromRequest(req)
			if routeID == uuid.Nil {
				return backend(ctx, req)
			}

			canary, err := b.configGetter.GetCanary(ctx, routeID)
			if err != nil {
				b.logger.Warn("canary config lookup failed",
					"error", err,
					"route_id", routeID,
				)
				// Fail open for canary (non-security feature) - route to primary.
				return backend(ctx, req)
			}
			if canary == nil || canary.CanaryWeight == 0 {
				return backend(ctx, req)
			}

			// Generate random number 0-99 using crypto/rand.
			selected, err := cryptoRandIntn(100)
			if err != nil {
				b.logger.Warn("canary random generation failed", "error", err)
				return backend(ctx, req)
			}

			var versionID uuid.UUID
			if selected < canary.CanaryWeight {
				versionID = canary.CanaryVersionID
			} else {
				versionID = canary.PrimaryVersionID
			}

			version, err := b.versionGetter.GetAPIVersion(ctx, versionID)
			if err != nil {
				b.logger.Warn("canary version lookup failed",
					"error", err,
					"version_id", versionID,
				)
				return backend(ctx, req)
			}

			// Store the selected version in context for downstream use.
			ctx = context.WithValue(ctx, canaryContextKey{}, version)
			return backend(ctx, req)
		}
	}
}

// cryptoRandIntn returns a cryptographically random int in [0, n).
func cryptoRandIntn(n int) (int, error) {
	max := big.NewInt(int64(n))
	result, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, err
	}
	return int(result.Int64()), nil
}

// routeIDFromRequest extracts the route UUID from the request.
// Checks the request headers first (set by gateway router), falls back to URL path.
func routeIDFromRequest(req *proxy.Request) uuid.UUID {
	if req.Headers != nil {
		if ids, ok := req.Headers["X-Route-Id"]; ok && len(ids) > 0 {
			if id, err := uuid.Parse(ids[0]); err == nil {
				return id
			}
		}
	}
	return uuid.Nil
}
