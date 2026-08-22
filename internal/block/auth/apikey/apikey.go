package apikey

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"github.com/luraproject/lura/v2/proxy"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"

	auth "github.com/mindfulcto-labs/apiloom-ce/internal/block/auth"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// defaultGracePeriod is the default time a deprecated key remains valid after rotation.
const defaultGracePeriod = 1 * time.Hour

// lastUsedChSize is the buffered channel capacity for async last-used updates.
const lastUsedChSize = 256

// authResult bundles the resolved consumer and API key for cache storage.
type authResult struct {
	consumer *domain.Consumer
	key      *domain.APIKey
}

// APIKeyBlock implements the Block interface for API key authentication.
type APIKeyBlock struct {
	config     APIKeyConfig
	store      ConsumerStore
	cache      *ttlcache.Cache[string, *domain.Consumer]
	keyCache   *ttlcache.Cache[string, *domain.APIKey]
	logger     *slog.Logger
	lastUsedCh chan uuid.UUID
	eg         *errgroup.Group
	cancel     context.CancelFunc
}

// NewAPIKeyBlock creates a new API key authentication block.
func NewAPIKeyBlock(store ConsumerStore, cfg APIKeyConfig) (*APIKeyBlock, error) {
	cfg.Validate()

	cache := ttlcache.New[string, *domain.Consumer](
		ttlcache.WithTTL[string, *domain.Consumer](cfg.CacheTTL),
	)
	go cache.Start()

	keyCache := ttlcache.New[string, *domain.APIKey](
		ttlcache.WithTTL[string, *domain.APIKey](cfg.CacheTTL),
	)
	go keyCache.Start()

	lastUsedCh := make(chan uuid.UUID, lastUsedChSize)
	ctx, cancel := context.WithCancel(context.Background())
	eg, egCtx := errgroup.WithContext(ctx)

	b := &APIKeyBlock{
		config:     cfg,
		store:      store,
		cache:      cache,
		keyCache:   keyCache,
		logger:     cfg.Logger,
		lastUsedCh: lastUsedCh,
		eg:         eg,
		cancel:     cancel,
	}

	// Single drain goroutine for last-used updates via errgroup.
	eg.Go(func() error {
		return b.drainLastUsed(egCtx)
	})

	return b, nil
}

// drainLastUsed processes last-used update events from the channel.
// It exits when the channel is closed or the context is cancelled.
func (b *APIKeyBlock) drainLastUsed(ctx context.Context) error {
	for {
		select {
		case id, ok := <-b.lastUsedCh:
			if !ok {
				return nil
			}
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := b.store.UpdateAPIKeyLastUsed(updateCtx, id); err != nil {
				b.logger.Warn("updating API key last used",
					slog.String("error", err.Error()),
				)
			}
			cancel()
		case <-ctx.Done():
			// Drain remaining items before exiting.
			for {
				select {
				case id, ok := <-b.lastUsedCh:
					if !ok {
						return nil
					}
					drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := b.store.UpdateAPIKeyLastUsed(drainCtx, id); err != nil {
						b.logger.Warn("draining API key last used",
							slog.String("error", err.Error()),
						)
					}
					cancel()
				default:
					return nil
				}
			}
		}
	}
}

// Name returns the block identifier.
func (b *APIKeyBlock) Name() string {
	return "auth-apikey"
}

// Middleware returns the Lura proxy middleware for API key authentication.
func (b *APIKeyBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) == 0 {
			return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				return nil, domain.NewProblemError(http.StatusInternalServerError, "no_next_proxy", "internal error")
			}
		}
		nextProxy := next[0]

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			rawKey := b.extractKey(req)
			if rawKey == "" {
				return nil, domain.NewProblemError(
					http.StatusUnauthorized, "missing_api_key", "API key required",
				)
			}

			if len(rawKey) < 8 {
				return nil, domain.NewProblemError(
					http.StatusUnauthorized, "invalid_api_key", "Invalid API key",
				)
			}

			prefix := rawKey[:8]

			var consumer *domain.Consumer
			var apiKey *domain.APIKey

			// Check cache first.
			if item := b.cache.Get(prefix); item != nil {
				consumer = item.Value()
				if keyItem := b.keyCache.Get(prefix); keyItem != nil {
					apiKey = keyItem.Value()
				}
			} else {
				// Cache miss: look up keys by prefix from store.
				var err error
				consumer, apiKey, err = b.validateKey(ctx, rawKey, prefix)
				if err != nil {
					return nil, err
				}

				// Cache the resolved consumer and key.
				b.cache.Set(prefix, consumer, ttlcache.DefaultTTL)
				if apiKey != nil {
					b.keyCache.Set(prefix, apiKey, ttlcache.DefaultTTL)
				}
			}

			// Scope enforcement: check if the key has permission for the target API.
			if apiKey != nil {
				if err := b.checkScope(ctx, req, apiKey); err != nil {
					return nil, err
				}
			}

			ctx = auth.SetConsumer(ctx, consumer)
			b.stripAPIKeyParam(req)
			return nextProxy(ctx, req)
		}
	}
}

// extractKey extracts the API key from the request header or query parameter.
// Header takes precedence over query parameter.
func (b *APIKeyBlock) extractKey(req *proxy.Request) string {
	if vals, ok := req.Headers[b.config.HeaderName]; ok && len(vals) > 0 && vals[0] != "" {
		return vals[0]
	}
	if b.config.AllowQueryParam {
		if key := req.Query.Get("api_key"); key != "" {
			return key
		}
	}
	return ""
}

// validateKey looks up API keys by prefix and validates using bcrypt.
// Returns the consumer and matched API key on success.
func (b *APIKeyBlock) validateKey(ctx context.Context, rawKey, prefix string) (*domain.Consumer, *domain.APIKey, error) {
	keys, err := b.store.GetAPIKeysByPrefix(ctx, prefix)
	if err != nil {
		b.logger.Error("looking up API keys",
			slog.String("prefix", prefix),
			slog.String("error", err.Error()),
		)
		// Fail closed per security rules.
		return nil, nil, domain.NewProblemError(
			http.StatusUnauthorized, "invalid_api_key", "Invalid API key",
		)
	}

	for i := range keys {
		ak := &keys[i]
		if bcrypt.CompareHashAndPassword([]byte(ak.KeyHash), []byte(rawKey)) != nil {
			continue
		}

		// Bcrypt match found. Check key status.
		if !ak.IsActive {
			b.logger.Warn("revoked API key used",
				slog.String("key_suffix", redactKey(rawKey)),
			)
			return nil, nil, domain.NewProblemError(
				http.StatusForbidden, "key_revoked", "API key has been revoked",
			)
		}

		if ak.ExpiresAt != nil && ak.ExpiresAt.Before(time.Now()) {
			b.logger.Warn("expired API key used",
				slog.String("key_suffix", redactKey(rawKey)),
			)
			return nil, nil, domain.NewProblemError(
				http.StatusUnauthorized, "key_expired", "API key has expired",
			)
		}

		// Check deprecated key grace period.
		if ak.DeprecatedAt != nil {
			gracePeriodEnd := ak.DeprecatedAt.Add(defaultGracePeriod)
			if time.Now().After(gracePeriodEnd) {
				b.logger.Warn("deprecated API key past grace period",
					slog.String("key_suffix", redactKey(rawKey)),
				)
				return nil, nil, domain.NewProblemError(
					http.StatusUnauthorized, "key_expired", "API key has been deprecated and grace period has expired",
				)
			}
		}

		// Key is valid. Fetch consumer.
		consumer, err := b.store.GetConsumer(ctx, ak.ConsumerID)
		if err != nil {
			b.logger.Error("loading consumer for API key",
				slog.String("key_suffix", redactKey(rawKey)),
				slog.String("error", err.Error()),
			)
			return nil, nil, domain.NewProblemError(
				http.StatusUnauthorized, "invalid_api_key", "Invalid API key",
			)
		}

		// Dispatch last-used update (non-blocking).
		select {
		case b.lastUsedCh <- ak.ID:
		default:
			b.logger.Warn("last-used channel full, dropping update",
				slog.String("key_suffix", redactKey(rawKey)),
			)
		}

		return consumer, ak, nil
	}

	return nil, nil, domain.NewProblemError(
		http.StatusUnauthorized, "invalid_api_key", "Invalid API key",
	)
}

// stripAPIKeyParam removes the api_key query parameter before forwarding upstream.
func (b *APIKeyBlock) stripAPIKeyParam(req *proxy.Request) {
	if req.Query.Has("api_key") {
		req.Query.Del("api_key")
		if req.URL != nil {
			req.URL.RawQuery = req.Query.Encode()
		}
	}
}

// checkScope validates that the API key has permission for the target route API.
// If no route API ID is in the context, scope check is skipped (backward compatible).
// Unrestricted keys (nil permissions) always pass.
func (b *APIKeyBlock) checkScope(ctx context.Context, req *proxy.Request, key *domain.APIKey) error {
	if !key.IsRestricted() {
		return nil
	}

	apiID := auth.GetRouteAPIID(ctx)
	if apiID == "" {
		return nil
	}

	action := methodToAction(req.Method)
	scope := "api:" + apiID + ":" + action

	if !key.HasScope(scope) {
		b.logger.Warn("API key lacks required scope",
			slog.String("required_scope", scope),
			slog.String("key_prefix", key.KeyPrefix),
		)
		return domain.NewProblemError(
			http.StatusForbidden, "insufficient_scope",
			fmt.Sprintf("API key lacks required scope: %s", scope),
		)
	}

	return nil
}

// methodToAction maps HTTP methods to permission actions (read or write).
func methodToAction(method string) string {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return "read"
	default:
		return "write"
	}
}

// Stop gracefully shuts down the block, draining pending last-used updates.
func (b *APIKeyBlock) Stop() error {
	close(b.lastUsedCh)
	b.cancel()
	err := b.eg.Wait()
	b.cache.Stop()
	b.keyCache.Stop()
	return err
}

// redactKey returns only the last 4 characters of an API key for safe logging.
func redactKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return fmt.Sprintf("...%s", key[len(key)-4:])
}
