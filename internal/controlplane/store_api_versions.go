package controlplane

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// APIVersionStore abstracts database operations for API versions and canary configs.
// Defined in the consumer package per project convention.
type APIVersionStore interface {
	ListAPIVersions(ctx context.Context, routeID uuid.UUID) ([]domain.APIVersion, error)
	CreateAPIVersion(ctx context.Context, version *domain.APIVersion) error
	GetAPIVersion(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error)
	SetCanary(ctx context.Context, canary *domain.CanaryConfig) error
	GetCanary(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error)
	ClearCanary(ctx context.Context, routeID uuid.UUID) error
}

// --- Postgres implementation ---

// ListAPIVersions returns all API versions for a route, ordered by created_at desc.
func (s *Store) ListAPIVersions(ctx context.Context, routeID uuid.UUID) ([]domain.APIVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, route_id, version_tag, config, changelog, is_breaking, created_at, created_by
		FROM api_versions
		WHERE route_id = $1
		ORDER BY created_at DESC`, routeID)
	if err != nil {
		return nil, fmt.Errorf("listing api versions: %w", err)
	}
	defer rows.Close()

	var versions []domain.APIVersion
	for rows.Next() {
		var v domain.APIVersion
		if err := rows.Scan(
			&v.ID, &v.RouteID, &v.VersionTag, &v.Config,
			&v.Changelog, &v.IsBreaking, &v.CreatedAt, &v.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scanning api version: %w", err)
		}
		versions = append(versions, v)
	}

	if versions == nil {
		versions = []domain.APIVersion{}
	}
	return versions, rows.Err()
}

// CreateAPIVersion inserts a new API version record.
func (s *Store) CreateAPIVersion(ctx context.Context, version *domain.APIVersion) error {
	if version.ID == uuid.Nil {
		version.ID = uuid.New()
	}
	now := time.Now().UTC()
	version.CreatedAt = now

	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_versions (id, route_id, version_tag, config, changelog, is_breaking, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		version.ID, version.RouteID, version.VersionTag, version.Config,
		version.Changelog, version.IsBreaking, version.CreatedAt, version.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("creating api version: %w", err)
	}
	return nil
}

// GetAPIVersion returns a single API version by ID.
func (s *Store) GetAPIVersion(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error) {
	var v domain.APIVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, route_id, version_tag, config, changelog, is_breaking, created_at, created_by
		FROM api_versions
		WHERE id = $1`, versionID).Scan(
		&v.ID, &v.RouteID, &v.VersionTag, &v.Config,
		&v.Changelog, &v.IsBreaking, &v.CreatedAt, &v.CreatedBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("api version %s: %w", versionID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("getting api version: %w", err)
	}
	return &v, nil
}

// SetCanary upserts the canary configuration for a route.
func (s *Store) SetCanary(ctx context.Context, canary *domain.CanaryConfig) error {
	now := time.Now().UTC()
	canary.UpdatedAt = now
	if canary.CreatedAt.IsZero() {
		canary.CreatedAt = now
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO canary_configs (route_id, primary_version_id, canary_version_id, canary_weight, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (route_id)
		DO UPDATE SET primary_version_id = $2, canary_version_id = $3, canary_weight = $4, updated_at = $6`,
		canary.RouteID, canary.PrimaryVersionID, canary.CanaryVersionID,
		canary.CanaryWeight, canary.CreatedAt, canary.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("setting canary: %w", err)
	}
	return nil
}

// GetCanary returns the active canary configuration for a route, or nil if none.
func (s *Store) GetCanary(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error) {
	var c domain.CanaryConfig
	err := s.pool.QueryRow(ctx, `
		SELECT route_id, primary_version_id, canary_version_id, canary_weight, created_at, updated_at
		FROM canary_configs
		WHERE route_id = $1`, routeID).Scan(
		&c.RouteID, &c.PrimaryVersionID, &c.CanaryVersionID,
		&c.CanaryWeight, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting canary: %w", err)
	}
	return &c, nil
}

// ClearCanary removes the canary configuration for a route.
func (s *Store) ClearCanary(ctx context.Context, routeID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM canary_configs WHERE route_id = $1`, routeID)
	if err != nil {
		return fmt.Errorf("clearing canary: %w", err)
	}
	return nil
}

// nopAPIVersionStore is a no-op implementation for testing.
type nopAPIVersionStore struct{}

func (n *nopAPIVersionStore) ListAPIVersions(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
	return []domain.APIVersion{}, nil
}
func (n *nopAPIVersionStore) CreateAPIVersion(_ context.Context, _ *domain.APIVersion) error {
	return nil
}
func (n *nopAPIVersionStore) GetAPIVersion(_ context.Context, _ uuid.UUID) (*domain.APIVersion, error) {
	return nil, domain.ErrNotFound
}
func (n *nopAPIVersionStore) SetCanary(_ context.Context, _ *domain.CanaryConfig) error {
	return nil
}
func (n *nopAPIVersionStore) GetCanary(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
	return nil, nil
}
func (n *nopAPIVersionStore) ClearCanary(_ context.Context, _ uuid.UUID) error {
	return nil
}
