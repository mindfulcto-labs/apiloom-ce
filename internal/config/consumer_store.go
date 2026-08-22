package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// bcryptCost is the cost factor for bcrypt hashing of API keys.
// Must be >= 12 per security rules.
const bcryptCost = 12

// CreateConsumer inserts a new consumer and returns the saved entity with generated ID.
func (s *ConfigStore) CreateConsumer(ctx context.Context, name string, metadata json.RawMessage) (*domain.Consumer, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating consumer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var c domain.Consumer
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO consumers (organization_id, name, metadata)
			 VALUES ($1, $2, $3)
			 RETURNING id, organization_id, name, metadata, is_active, created_at, updated_at`,
			orgID, name, metadata,
		).Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Metadata, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("consumer name %q: %w", name, domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting consumer: %w", err)
	}

	return &c, nil
}

// GetConsumer returns a consumer by ID. Returns domain.ErrNotFound if no consumer exists.
func (s *ConfigStore) GetConsumer(ctx context.Context, id uuid.UUID) (*domain.Consumer, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting consumer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var c domain.Consumer
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, name, metadata, is_active, created_at, updated_at
			 FROM consumers
			 WHERE id = $1`,
			id,
		).Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Metadata, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("consumer %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading consumer %s: %w", id, err)
	}

	return &c, nil
}

// ListConsumers returns all active consumers ordered by name.
func (s *ConfigStore) ListConsumers(ctx context.Context) ([]domain.Consumer, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing consumers: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var consumers []domain.Consumer
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		rows, qErr := tx.Query(ctx,
			`SELECT id, organization_id, name, metadata, is_active, created_at, updated_at
			 FROM consumers
			 WHERE is_active = true
			 ORDER BY name`,
		)
		if qErr != nil {
			return fmt.Errorf("querying consumers: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var c domain.Consumer
			if scanErr := rows.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Metadata, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning consumer: %w", scanErr)
			}
			consumers = append(consumers, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing consumers: %w", err)
	}

	return consumers, nil
}

// DeleteConsumer soft-deletes a consumer by setting is_active to false.
func (s *ConfigStore) DeleteConsumer(ctx context.Context, id uuid.UUID) error {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting consumer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`UPDATE consumers SET is_active = false, updated_at = NOW() WHERE id = $1 AND is_active = true`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting consumer %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("consumer %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// GetAPIKey returns a single API key by ID. Returns domain.ErrNotFound if not found.
func (s *ConfigStore) GetAPIKey(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting API key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ak domain.APIKey
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, permissions, created_at
			 FROM api_keys WHERE id = $1`,
			id,
		).Scan(&ak.ID, &ak.OrganizationID, &ak.ConsumerID, &ak.KeyHash, &ak.KeyPrefix, &ak.IsActive,
			&ak.DeprecatedAt, &ak.ExpiresAt, &ak.LastUsedAt, &ak.RateLimitOverride, &ak.Permissions, &ak.CreatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("API key %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading API key %s: %w", id, err)
	}

	return &ak, nil
}

// CreateAPIKey generates a new API key for the given consumer using crypto/rand.
// Returns the raw key (hex-encoded, 64 chars) exactly once and the saved APIKey entity.
// The raw key is never stored -- only its bcrypt hash is persisted.
func (s *ConfigStore) CreateAPIKey(ctx context.Context, consumerID uuid.UUID, permissions json.RawMessage) (string, *domain.APIKey, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("creating API key: %w", err)
	}

	rawKey, keyHash, keyPrefix, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generating API key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ak domain.APIKey
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO api_keys (organization_id, consumer_id, key_hash, key_prefix, permissions)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, permissions, created_at`,
			orgID, consumerID, keyHash, keyPrefix, permissions,
		).Scan(&ak.ID, &ak.OrganizationID, &ak.ConsumerID, &ak.KeyHash, &ak.KeyPrefix, &ak.IsActive,
			&ak.DeprecatedAt, &ak.ExpiresAt, &ak.LastUsedAt, &ak.RateLimitOverride, &ak.Permissions, &ak.CreatedAt)
	})
	if err != nil {
		return "", nil, fmt.Errorf("inserting API key: %w", err)
	}

	return rawKey, &ak, nil
}

// GetAPIKeysByPrefix returns active API keys matching the given prefix for bcrypt comparison.
func (s *ConfigStore) GetAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting API keys by prefix: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var keys []domain.APIKey
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		rows, qErr := tx.Query(ctx,
			`SELECT id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, permissions, created_at
			 FROM api_keys
			 WHERE key_prefix = $1 AND is_active = true`,
			prefix,
		)
		if qErr != nil {
			return fmt.Errorf("querying API keys by prefix: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var ak domain.APIKey
			if scanErr := rows.Scan(&ak.ID, &ak.OrganizationID, &ak.ConsumerID, &ak.KeyHash, &ak.KeyPrefix, &ak.IsActive,
				&ak.DeprecatedAt, &ak.ExpiresAt, &ak.LastUsedAt, &ak.RateLimitOverride, &ak.Permissions, &ak.CreatedAt); scanErr != nil {
				return fmt.Errorf("scanning API key: %w", scanErr)
			}
			keys = append(keys, ak)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("getting API keys by prefix: %w", err)
	}

	return keys, nil
}

// RevokeAPIKey immediately invalidates an API key by setting is_active=false.
func (s *ConfigStore) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("revoking API key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`UPDATE api_keys SET is_active = false WHERE id = $1`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("revoking API key %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("API key %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key.
func (s *ConfigStore) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("updating API key last used: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx,
			`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`,
			id,
		)
		if execErr != nil {
			return fmt.Errorf("updating last used for API key %s: %w", id, execErr)
		}
		return nil
	})
}

// RotateAPIKey atomically creates a new key for the same consumer and marks the old key
// as deprecated. The old key remains active (is_active=true) with deprecated_at set.
// The auth block checks deprecated_at + gracePeriod to determine if the old key is still valid.
func (s *ConfigStore) RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error) {
	_ = gracePeriod // Stored for documentation; auth block enforces the grace period check.

	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("rotating API key: %w", err)
	}

	rawKey, keyHash, keyPrefix, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generating new API key for rotation: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var newKey domain.APIKey
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		// Step 1: Get old key to find consumer_id and verify it's active.
		var consumerID uuid.UUID
		var oldIsActive bool
		qErr := tx.QueryRow(ctx,
			`SELECT consumer_id, is_active FROM api_keys WHERE id = $1`,
			oldKeyID,
		).Scan(&consumerID, &oldIsActive)
		if qErr != nil {
			if errors.Is(qErr, pgx.ErrNoRows) {
				return fmt.Errorf("old API key %s: %w", oldKeyID, domain.ErrNotFound)
			}
			return fmt.Errorf("loading old API key %s: %w", oldKeyID, qErr)
		}
		if !oldIsActive {
			return fmt.Errorf("old API key %s is not active: %w", oldKeyID, domain.ErrNotFound)
		}

		// Step 2: Deprecate old key.
		_, execErr := tx.Exec(ctx,
			`UPDATE api_keys SET deprecated_at = NOW() WHERE id = $1`,
			oldKeyID,
		)
		if execErr != nil {
			return fmt.Errorf("deprecating old API key %s: %w", oldKeyID, execErr)
		}

		// Step 3: Insert new key for same consumer, copying permissions from old key.
		var oldPermissions json.RawMessage
		_ = tx.QueryRow(ctx,
			`SELECT permissions FROM api_keys WHERE id = $1`,
			oldKeyID,
		).Scan(&oldPermissions)

		return tx.QueryRow(ctx,
			`INSERT INTO api_keys (organization_id, consumer_id, key_hash, key_prefix, permissions)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, permissions, created_at`,
			orgID, consumerID, keyHash, keyPrefix, oldPermissions,
		).Scan(&newKey.ID, &newKey.OrganizationID, &newKey.ConsumerID, &newKey.KeyHash, &newKey.KeyPrefix, &newKey.IsActive,
			&newKey.DeprecatedAt, &newKey.ExpiresAt, &newKey.LastUsedAt, &newKey.RateLimitOverride, &newKey.Permissions, &newKey.CreatedAt)
	})
	if err != nil {
		return "", nil, fmt.Errorf("rotating API key: %w", err)
	}

	return rawKey, &newKey, nil
}

// generateAPIKey creates a new random API key using crypto/rand.
// Returns the raw hex key (64 chars), its bcrypt hash, and the prefix (first 8 chars).
func generateAPIKey() (rawKey, keyHash, keyPrefix string, err error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", "", fmt.Errorf("reading crypto/rand: %w", err)
	}

	rawKey = hex.EncodeToString(keyBytes)
	keyPrefix = rawKey[:8]

	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcryptCost)
	if err != nil {
		return "", "", "", fmt.Errorf("hashing API key: %w", err)
	}
	keyHash = string(hash)

	return rawKey, keyHash, keyPrefix, nil
}
