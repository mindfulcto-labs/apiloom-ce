// Package auth provides authentication block implementations and consumer context helpers.
package auth

import (
	"context"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// ctxKeyConsumer is the unexported typed context key for consumer identity.
type ctxKeyConsumer struct{}

// ctxKeyAPIKey is the unexported typed context key for the authenticated API key.
type ctxKeyAPIKey struct{}

// ctxKeyRouteAPIID is the unexported typed context key for the route's API identifier.
type ctxKeyRouteAPIID struct{}

// SetConsumer attaches a consumer to the context for downstream handlers.
func SetConsumer(ctx context.Context, consumer *domain.Consumer) context.Context {
	return context.WithValue(ctx, ctxKeyConsumer{}, consumer)
}

// GetConsumer retrieves the consumer from the context. Returns nil if no
// consumer is present or if the value has the wrong type.
func GetConsumer(ctx context.Context) *domain.Consumer {
	v, ok := ctx.Value(ctxKeyConsumer{}).(*domain.Consumer)
	if !ok {
		return nil
	}
	return v
}

// SetAPIKey attaches an API key to the context for downstream handlers
// (e.g., rate limit override resolution).
func SetAPIKey(ctx context.Context, key *domain.APIKey) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKey{}, key)
}

// GetAPIKey retrieves the authenticated API key from the context.
// Returns nil if no API key is present or if the value has the wrong type.
func GetAPIKey(ctx context.Context) *domain.APIKey {
	v, ok := ctx.Value(ctxKeyAPIKey{}).(*domain.APIKey)
	if !ok {
		return nil
	}
	return v
}

// SetRouteAPIID attaches the route's API identifier to the context.
// This comes from the Lura route ExtraConfig at key "flexgateway/api_id".
func SetRouteAPIID(ctx context.Context, apiID string) context.Context {
	return context.WithValue(ctx, ctxKeyRouteAPIID{}, apiID)
}

// GetRouteAPIID retrieves the route API ID from the context.
// Returns empty string if not set.
func GetRouteAPIID(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyRouteAPIID{}).(string)
	if !ok {
		return ""
	}
	return v
}
