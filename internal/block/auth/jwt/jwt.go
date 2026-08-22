package jwt

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mindfulcto-labs/apiloom-ce/internal/block/auth"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/luraproject/lura/v2/proxy"
)

// Option configures optional JWTBlock parameters.
type Option func(*JWTBlock)

// WithLogger sets a custom logger for the JWTBlock.
func WithLogger(l *slog.Logger) Option {
	return func(b *JWTBlock) {
		b.logger = l
	}
}

// JWTBlock implements the gateway.Block interface for JWT validation.
type JWTBlock struct {
	config JWTConfig
	cache  *jwk.Cache
	logger *slog.Logger
}

// NewJWTBlock creates a new JWT validation block. It validates the config,
// initializes the JWKS cache, and performs an initial key fetch.
// The ctx parameter should be the server lifecycle context (not per-request).
func NewJWTBlock(ctx context.Context, cfg JWTConfig, opts ...Option) (*JWTBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.defaults()

	b := &JWTBlock{
		config: cfg,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}

	// Initialize JWKS cache with server lifecycle context (NOT per-request context).
	cache := jwk.NewCache(ctx)
	err := cache.Register(cfg.JWKSEndpoint,
		jwk.WithMinRefreshInterval(cfg.RefreshInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("registering jwks endpoint: %w", err)
	}
	b.cache = cache

	// Force initial fetch. Log warning on failure but do not crash.
	_, err = cache.Refresh(ctx, cfg.JWKSEndpoint)
	if err != nil {
		b.logger.Warn("initial JWKS fetch failed", "endpoint", cfg.JWKSEndpoint, "error", err)
	}

	return b, nil
}

// Name returns the block identifier.
func (b *JWTBlock) Name() string {
	return "auth-jwt"
}

// Middleware returns a Lura proxy middleware that validates JWT tokens.
func (b *JWTBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			// 1. Extract Bearer token from Authorization header.
			tokenStr, err := extractBearerToken(req)
			if err != nil {
				return nil, err
			}

			// 2. Get keyset from cache.
			keyset, err := b.cache.Get(ctx, b.config.JWKSEndpoint)
			if err != nil {
				b.logger.Error("fetching JWKS keyset", "error", err)
				return nil, domain.NewProblemError(
					http.StatusUnauthorized, "invalid_token", "Token verification failed",
				)
			}

			// 3. Parse and validate the token.
			token, err := b.parseToken(ctx, tokenStr, keyset)
			if err != nil {
				return nil, err
			}

			// 4. Build Consumer and attach to context.
			sub := token.Subject()
			consumer := &domain.Consumer{
				ID:       uuid.New(),
				Name:     sub,
				IsActive: true,
			}
			ctx = auth.SetConsumer(ctx, consumer)

			// 5. Extract configured claims into request headers.
			b.extractClaims(token, req)

			// 6. Forward to next proxy.
			return nextProxy(ctx, req)
		}
	}
}

// extractBearerToken pulls the Bearer token from the Authorization header.
func extractBearerToken(req *proxy.Request) (string, error) {
	authHeaders := req.Headers["Authorization"]
	if len(authHeaders) == 0 {
		return "", domain.NewProblemError(
			http.StatusUnauthorized, "missing_token", "Authorization header is required",
		)
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", domain.NewProblemError(
			http.StatusUnauthorized, "missing_token", "Authorization header must use Bearer scheme",
		)
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", domain.NewProblemError(
			http.StatusUnauthorized, "missing_token", "Bearer token is empty",
		)
	}

	return token, nil
}

// parseToken validates the JWT token signature, expiry, issuer, and audience.
// On unknown kid, it refreshes the cache once and retries.
func (b *JWTBlock) parseToken(ctx context.Context, tokenStr string, keyset jwk.Set) (jwt.Token, error) {
	parseOpts := b.buildParseOptions(keyset)

	token, err := jwt.Parse([]byte(tokenStr), parseOpts...)
	if err != nil {
		// On failure, attempt a single JWKS refresh and retry (handles unknown kid).
		refreshedSet, refreshErr := b.cache.Refresh(ctx, b.config.JWKSEndpoint)
		if refreshErr != nil {
			return nil, domain.NewProblemError(
				http.StatusUnauthorized, "invalid_token", "Token verification failed",
			)
		}

		retryOpts := b.buildParseOptions(refreshedSet)
		token, err = jwt.Parse([]byte(tokenStr), retryOpts...)
		if err != nil {
			return nil, domain.NewProblemError(
				http.StatusUnauthorized, "invalid_token", "Token verification failed",
			)
		}
	}

	return token, nil
}

// buildParseOptions constructs the jwt.Parse options for signature and claim validation.
func (b *JWTBlock) buildParseOptions(keyset jwk.Set) []jwt.ParseOption {
	// Build allowed algorithms set for filtering.
	algSet := make(map[jwa.SignatureAlgorithm]struct{}, len(b.config.AllowedAlgorithms))
	for _, a := range b.config.AllowedAlgorithms {
		algSet[jwa.SignatureAlgorithm(a)] = struct{}{}
	}

	// Filter keyset to only allowed algorithms.
	filtered := jwk.NewSet()
	iter := keyset.Keys(context.Background())
	for iter.Next(context.Background()) {
		pair := iter.Pair()
		key, ok := pair.Value.(jwk.Key)
		if !ok {
			continue
		}
		if _, allowed := algSet[key.Algorithm().(jwa.SignatureAlgorithm)]; allowed {
			_ = filtered.AddKey(key)
		}
	}

	opts := []jwt.ParseOption{
		jwt.WithKeySet(filtered),
		jwt.WithValidate(true),
	}

	// Add issuer validation -- at least one must match.
	for _, iss := range b.config.AllowedIssuers {
		opts = append(opts, jwt.WithIssuer(iss))
	}

	// Add audience validation -- at least one must match.
	for _, aud := range b.config.AllowedAudiences {
		opts = append(opts, jwt.WithAudience(aud))
	}

	return opts
}

// extractClaims maps allowlisted token claims to request headers.
func (b *JWTBlock) extractClaims(token jwt.Token, req *proxy.Request) {
	claims, _ := token.AsMap(context.Background())
	for claimName, headerName := range b.config.ClaimMap {
		val, exists := claims[claimName]
		if !exists {
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		if strVal == "" {
			continue
		}
		req.Headers[headerName] = []string{strVal}
	}
}
