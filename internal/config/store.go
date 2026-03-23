package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// ConfigStore provides CRUD operations for gateway configuration backed by Postgres.
type ConfigStore struct {
	pool *pgxpool.Pool
}

// NewConfigStore creates a new ConfigStore with the given connection pool.
func NewConfigStore(pool *pgxpool.Pool) *ConfigStore {
	return &ConfigStore{pool: pool}
}

// RunMigrations applies embedded SQL migrations to the database.
// Returns nil if migrations are already up to date.
func RunMigrations(ctx context.Context, dsn string) error {
	src, err := iofs.New(Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer func() {
		m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// SaveRoute validates and persists a RouteConfig. Computes checksum automatically.
// Returns the saved entity with generated ID and timestamps.
// The organization ID is extracted from the Go context.
func (s *ConfigStore) SaveRoute(ctx context.Context, rc *domain.RouteConfig) (*domain.RouteConfig, error) {
	if err := ValidateRouteConfig(rc); err != nil {
		return nil, err
	}

	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("saving route: %w", err)
	}

	rc.Checksum = domain.ComputeChecksum(rc.Config)
	if rc.Version == 0 {
		rc.Version = 1
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var saved domain.RouteConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO routes (organization_id, name, version, config, metadata, checksum, is_active)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at`,
			orgID, rc.Name, rc.Version, rc.Config, rc.Metadata, rc.Checksum, true,
		).Scan(
			&saved.ID, &saved.OrganizationID, &saved.Name, &saved.Version, &saved.Config,
			&saved.Metadata, &saved.Checksum, &saved.IsActive,
			&saved.CreatedAt, &saved.UpdatedAt,
		)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("route name %q: %w", rc.Name, domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting route: %w", err)
	}

	return &saved, nil
}

// LoadRoutes returns all active route configurations for the current organization.
func (s *ConfigStore) LoadRoutes(ctx context.Context) ([]domain.RouteConfig, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading routes: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var routes []domain.RouteConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		rows, qErr := tx.Query(ctx,
			`SELECT id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at
			 FROM routes
			 WHERE is_active = true
			 ORDER BY name`,
		)
		if qErr != nil {
			return fmt.Errorf("querying routes: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.RouteConfig
			if scanErr := rows.Scan(
				&r.ID, &r.OrganizationID, &r.Name, &r.Version, &r.Config,
				&r.Metadata, &r.Checksum, &r.IsActive,
				&r.CreatedAt, &r.UpdatedAt,
			); scanErr != nil {
				return fmt.Errorf("scanning route: %w", scanErr)
			}
			routes = append(routes, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("loading routes: %w", err)
	}

	return routes, nil
}

// LoadRoute returns a single route by ID. Returns domain.ErrNotFound if no route exists.
func (s *ConfigStore) LoadRoute(ctx context.Context, id uuid.UUID) (*domain.RouteConfig, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading route: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var r domain.RouteConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at
			 FROM routes
			 WHERE id = $1`,
			id,
		).Scan(
			&r.ID, &r.OrganizationID, &r.Name, &r.Version, &r.Config,
			&r.Metadata, &r.Checksum, &r.IsActive,
			&r.CreatedAt, &r.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading route %s: %w", id, err)
	}

	return &r, nil
}

// DeleteRoute soft-deletes a route by setting is_active to false.
func (s *ConfigStore) DeleteRoute(ctx context.Context, id uuid.UUID) error {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting route: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`UPDATE routes SET is_active = false WHERE id = $1 AND is_active = true`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting route %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// SaveBackend validates and persists a BackendConfig.
// Returns the saved entity with generated ID and timestamps.
func (s *ConfigStore) SaveBackend(ctx context.Context, bc *domain.BackendConfig) (*domain.BackendConfig, error) {
	if err := ValidateBackendConfig(bc); err != nil {
		return nil, err
	}

	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("saving backend: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var saved domain.BackendConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO backends (organization_id, name, config, metadata)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, organization_id, name, config, metadata, created_at, updated_at`,
			orgID, bc.Name, bc.Config, bc.Metadata,
		).Scan(
			&saved.ID, &saved.OrganizationID, &saved.Name, &saved.Config,
			&saved.Metadata, &saved.CreatedAt, &saved.UpdatedAt,
		)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("backend name %q: %w", bc.Name, domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting backend: %w", err)
	}

	return &saved, nil
}

// LoadBackends returns all backend configurations for the current organization.
func (s *ConfigStore) LoadBackends(ctx context.Context) ([]domain.BackendConfig, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading backends: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var backends []domain.BackendConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		rows, qErr := tx.Query(ctx,
			`SELECT id, organization_id, name, config, metadata, created_at, updated_at
			 FROM backends
			 ORDER BY name`,
		)
		if qErr != nil {
			return fmt.Errorf("querying backends: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var b domain.BackendConfig
			if scanErr := rows.Scan(
				&b.ID, &b.OrganizationID, &b.Name, &b.Config,
				&b.Metadata, &b.CreatedAt, &b.UpdatedAt,
			); scanErr != nil {
				return fmt.Errorf("scanning backend: %w", scanErr)
			}
			backends = append(backends, b)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("loading backends: %w", err)
	}

	return backends, nil
}

// LoadBackend returns a single backend by ID. Returns domain.ErrNotFound if not found.
func (s *ConfigStore) LoadBackend(ctx context.Context, id uuid.UUID) (*domain.BackendConfig, error) {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading backend: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var b domain.BackendConfig
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, name, config, metadata, created_at, updated_at
			 FROM backends
			 WHERE id = $1`,
			id,
		).Scan(
			&b.ID, &b.OrganizationID, &b.Name, &b.Config,
			&b.Metadata, &b.CreatedAt, &b.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading backend %s: %w", id, err)
	}

	return &b, nil
}

// DeleteBackend removes a backend by ID.
func (s *ConfigStore) DeleteBackend(ctx context.Context, id uuid.UUID) error {
	orgID, err := OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting backend: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`DELETE FROM backends WHERE id = $1`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting backend %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// LoadGatewayConfig assembles a full GatewayConfig from all active routes and backends.
func (s *ConfigStore) LoadGatewayConfig(ctx context.Context) (*domain.GatewayConfig, error) {
	routes, err := s.LoadRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading routes for gateway config: %w", err)
	}

	backends, err := s.LoadBackends(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading backends for gateway config: %w", err)
	}

	return &domain.GatewayConfig{
		Version:  1,
		Routes:   routes,
		Backends: backends,
		LoadedAt: time.Now(),
	}, nil
}
