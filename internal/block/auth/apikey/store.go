// Package apikey implements the API key authentication block.
package apikey

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// ConsumerStore is the interface for consumer and API key persistence.
// Defined in the consumer package (apikey) per CLAUDE.md "interfaces in consumer" rule.
// config.ConfigStore satisfies this interface implicitly via duck typing.
type ConsumerStore interface {
	GetAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error)
	GetConsumer(ctx context.Context, id uuid.UUID) (*domain.Consumer, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
	RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error)
}
