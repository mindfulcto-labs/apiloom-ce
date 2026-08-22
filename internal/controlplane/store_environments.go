package controlplane

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// EnvironmentStore abstracts database operations for environments.
// Defined in the consumer package per project convention.
type EnvironmentStore interface {
	ListEnvironments(ctx context.Context, orgID uuid.UUID) ([]domain.Environment, error)
	GetEnvironment(ctx context.Context, orgID, envID uuid.UUID) (*domain.Environment, error)
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, orgID, envID uuid.UUID) error
	PromoteEnvironment(ctx context.Context, orgID, sourceEnvID, targetEnvID uuid.UUID) (*domain.PromotionResult, error)
	CountEnvironments(ctx context.Context, orgID uuid.UUID) (int, error)
}

// --- Postgres implementation ---

// ListEnvironments returns all environments for an organization, ordered by sort_order.
func (s *Store) ListEnvironments(ctx context.Context, orgID uuid.UUID) ([]domain.Environment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.organization_id, e.name, e.slug, e.description,
		       e.is_production, e.sort_order, e.created_at, e.updated_at,
		       0 AS route_count
		FROM environments e
		WHERE e.organization_id = $1
		ORDER BY e.sort_order, e.created_at`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer rows.Close()

	var envs []domain.Environment
	for rows.Next() {
		var env domain.Environment
		if err := rows.Scan(
			&env.ID, &env.OrganizationID, &env.Name, &env.Slug, &env.Description,
			&env.IsProduction, &env.SortOrder, &env.CreatedAt, &env.UpdatedAt,
			&env.RouteCount,
		); err != nil {
			return nil, fmt.Errorf("scanning environment: %w", err)
		}
		envs = append(envs, env)
	}

	if envs == nil {
		envs = []domain.Environment{}
	}
	return envs, rows.Err()
}

// GetEnvironment returns a single environment by org and ID.
func (s *Store) GetEnvironment(ctx context.Context, orgID, envID uuid.UUID) (*domain.Environment, error) {
	var env domain.Environment
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, description,
		       is_production, sort_order, created_at, updated_at
		FROM environments
		WHERE organization_id = $1 AND id = $2`, orgID, envID).Scan(
		&env.ID, &env.OrganizationID, &env.Name, &env.Slug, &env.Description,
		&env.IsProduction, &env.SortOrder, &env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("environment %s: %w", envID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("getting environment: %w", err)
	}
	return &env, nil
}

// CreateEnvironment inserts a new environment after checking the per-org limit.
func (s *Store) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	orgID := env.OrganizationID
	if orgID == uuid.Nil {
		var err error
		orgID, err = config.OrgFromContext(ctx)
		if err != nil {
			return fmt.Errorf("resolving org: %w", err)
		}
		env.OrganizationID = orgID
	}

	count, err := s.CountEnvironments(ctx, orgID)
	if err != nil {
		return err
	}
	if count >= domain.MaxEnvironmentsPerOrg {
		return fmt.Errorf("maximum %d environments per organization: %w", domain.MaxEnvironmentsPerOrg, domain.ErrInvalidConfig)
	}

	if env.ID == uuid.Nil {
		env.ID = uuid.New()
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now

	_, err = s.pool.Exec(ctx, `
		INSERT INTO environments (id, organization_id, name, slug, description, is_production, sort_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		env.ID, env.OrganizationID, env.Name, env.Slug, env.Description,
		env.IsProduction, env.SortOrder, env.CreatedAt, env.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating environment: %w", err)
	}
	return nil
}

// DeleteEnvironment removes an environment. Production environments cannot be deleted.
func (s *Store) DeleteEnvironment(ctx context.Context, orgID, envID uuid.UUID) error {
	// Check if environment is production.
	env, err := s.GetEnvironment(ctx, orgID, envID)
	if err != nil {
		return err
	}
	if env.IsProduction {
		return fmt.Errorf("cannot delete production environment: %w", domain.ErrForbidden)
	}

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM environments
		WHERE organization_id = $1 AND id = $2`, orgID, envID)
	if err != nil {
		return fmt.Errorf("deleting environment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("environment %s: %w", envID, domain.ErrNotFound)
	}
	return nil
}

// PromoteEnvironment copies routes and backends from the source environment
// to the target environment within a transaction.
func (s *Store) PromoteEnvironment(ctx context.Context, orgID, sourceEnvID, targetEnvID uuid.UUID) (*domain.PromotionResult, error) {
	if sourceEnvID == targetEnvID {
		return nil, fmt.Errorf("source and target environments must differ: %w", domain.ErrInvalidConfig)
	}

	// Verify both environments exist and belong to the org.
	if _, err := s.GetEnvironment(ctx, orgID, sourceEnvID); err != nil {
		return nil, fmt.Errorf("source environment: %w", err)
	}
	if _, err := s.GetEnvironment(ctx, orgID, targetEnvID); err != nil {
		return nil, fmt.Errorf("target environment: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning promotion tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result := &domain.PromotionResult{}

	// Promote routes: upsert by name within target env.
	rows, err := tx.Query(ctx, `
		SELECT id, name, version, config, metadata, checksum, is_active
		FROM routes
		WHERE organization_id = $1 AND environment_id = $2`, orgID, sourceEnvID)
	if err != nil {
		return nil, fmt.Errorf("reading source routes: %w", err)
	}
	defer rows.Close()

	type routeRow struct {
		id       uuid.UUID
		name     string
		version  int
		config   []byte
		metadata []byte
		checksum string
		isActive bool
	}
	var sourceRoutes []routeRow
	for rows.Next() {
		var r routeRow
		if err := rows.Scan(&r.id, &r.name, &r.version, &r.config, &r.metadata, &r.checksum, &r.isActive); err != nil {
			return nil, fmt.Errorf("scanning source route: %w", err)
		}
		sourceRoutes = append(sourceRoutes, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source routes: %w", err)
	}

	for _, r := range sourceRoutes {
		// Check if a route with same name exists in target env.
		var existingID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT id FROM routes
			WHERE organization_id = $1 AND environment_id = $2 AND name = $3`,
			orgID, targetEnvID, r.name).Scan(&existingID)

		if err == pgx.ErrNoRows {
			// Create new route in target env.
			newID := uuid.New()
			now := time.Now().UTC()
			_, err = tx.Exec(ctx, `
				INSERT INTO routes (id, organization_id, environment_id, name, version, config, metadata, checksum, is_active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				newID, orgID, targetEnvID, r.name, 1, r.config, r.metadata, r.checksum, r.isActive, now, now)
			if err != nil {
				return nil, fmt.Errorf("creating promoted route %q: %w", r.name, err)
			}
			result.RoutesCreated++
		} else if err != nil {
			return nil, fmt.Errorf("checking existing route %q: %w", r.name, err)
		} else {
			// Update existing route.
			now := time.Now().UTC()
			_, err = tx.Exec(ctx, `
				UPDATE routes SET config = $1, metadata = $2, checksum = $3, is_active = $4,
				       version = version + 1, updated_at = $5
				WHERE id = $6`,
				r.config, r.metadata, r.checksum, r.isActive, now, existingID)
			if err != nil {
				return nil, fmt.Errorf("updating promoted route %q: %w", r.name, err)
			}
			result.RoutesUpdated++
		}
	}

	// Promote backends: upsert by name within target env.
	bRows, err := tx.Query(ctx, `
		SELECT id, name, config, metadata
		FROM backends
		WHERE organization_id = $1 AND environment_id = $2`, orgID, sourceEnvID)
	if err != nil {
		return nil, fmt.Errorf("reading source backends: %w", err)
	}
	defer bRows.Close()

	type backendRow struct {
		id       uuid.UUID
		name     string
		config   []byte
		metadata []byte
	}
	var sourceBackends []backendRow
	for bRows.Next() {
		var b backendRow
		if err := bRows.Scan(&b.id, &b.name, &b.config, &b.metadata); err != nil {
			return nil, fmt.Errorf("scanning source backend: %w", err)
		}
		sourceBackends = append(sourceBackends, b)
	}
	if err := bRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source backends: %w", err)
	}

	for _, b := range sourceBackends {
		var existingID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT id FROM backends
			WHERE organization_id = $1 AND environment_id = $2 AND name = $3`,
			orgID, targetEnvID, b.name).Scan(&existingID)

		if err == pgx.ErrNoRows {
			newID := uuid.New()
			now := time.Now().UTC()
			_, err = tx.Exec(ctx, `
				INSERT INTO backends (id, organization_id, environment_id, name, config, metadata, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				newID, orgID, targetEnvID, b.name, b.config, b.metadata, now, now)
			if err != nil {
				return nil, fmt.Errorf("creating promoted backend %q: %w", b.name, err)
			}
			result.BackendsCreated++
		} else if err != nil {
			return nil, fmt.Errorf("checking existing backend %q: %w", b.name, err)
		} else {
			now := time.Now().UTC()
			_, err = tx.Exec(ctx, `
				UPDATE backends SET config = $1, metadata = $2, updated_at = $3
				WHERE id = $4`,
				b.config, b.metadata, now, existingID)
			if err != nil {
				return nil, fmt.Errorf("updating promoted backend %q: %w", b.name, err)
			}
			result.BackendsUpdated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing promotion tx: %w", err)
	}
	return result, nil
}

// CountEnvironments returns the number of environments for an org.
func (s *Store) CountEnvironments(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM environments WHERE organization_id = $1`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting environments: %w", err)
	}
	return count, nil
}

// nopEnvironmentStore is a no-op implementation for testing.
type nopEnvironmentStore struct{}

func (n *nopEnvironmentStore) ListEnvironments(_ context.Context, _ uuid.UUID) ([]domain.Environment, error) {
	return []domain.Environment{}, nil
}
func (n *nopEnvironmentStore) GetEnvironment(_ context.Context, _, _ uuid.UUID) (*domain.Environment, error) {
	return nil, domain.ErrNotFound
}
func (n *nopEnvironmentStore) CreateEnvironment(_ context.Context, _ *domain.Environment) error {
	return nil
}
func (n *nopEnvironmentStore) DeleteEnvironment(_ context.Context, _, _ uuid.UUID) error {
	return nil
}
func (n *nopEnvironmentStore) PromoteEnvironment(_ context.Context, _, _, _ uuid.UUID) (*domain.PromotionResult, error) {
	return &domain.PromotionResult{}, nil
}
func (n *nopEnvironmentStore) CountEnvironments(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
