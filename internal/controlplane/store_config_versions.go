package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// ConfigVersion represents a point-in-time snapshot of the gateway configuration.
type ConfigVersion struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Version        int             `json:"version"`
	Snapshot       json.RawMessage `json:"snapshot"`
	Author         string          `json:"author"`
	Description    string          `json:"description"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ConfigVersionStore abstracts config version operations for handler testability.
type ConfigVersionStore interface {
	CreateVersion(ctx context.Context, snapshot json.RawMessage, author, description string) (*ConfigVersion, error)
	ListVersions(ctx context.Context, params ListParams) (*PagedResponse[ConfigVersion], error)
	GetVersion(ctx context.Context, version int) (*ConfigVersion, error)
	GetLatestVersion(ctx context.Context) (*ConfigVersion, error)
	RollbackToVersion(ctx context.Context, targetVersion int) (*ConfigVersion, error)
}

// CreateVersion inserts a new config version with auto-incrementing version number per org.
func (s *Store) CreateVersion(ctx context.Context, snapshot json.RawMessage, author, description string) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating config version: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var cv ConfigVersion
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO config_versions (organization_id, version, snapshot, author, description)
			 SELECT $1, COALESCE(MAX(version), 0) + 1, $2, $3, $4
			 FROM config_versions
			 WHERE organization_id = $1
			 RETURNING id, organization_id, version, snapshot, author, description, created_at`,
			orgID, snapshot, author, description,
		).Scan(&cv.ID, &cv.OrganizationID, &cv.Version, &cv.Snapshot, &cv.Author, &cv.Description, &cv.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("creating config version: %w", err)
	}

	return &cv, nil
}

// ListVersions returns paginated config versions for an org, newest first.
func (s *Store) ListVersions(ctx context.Context, params ListParams) (*PagedResponse[ConfigVersion], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing config versions: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var versions []ConfigVersion
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, version, snapshot, author, description, created_at
				 FROM config_versions
				 ORDER BY created_at DESC, id DESC
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, version, snapshot, author, description, created_at
				 FROM config_versions
				 WHERE (created_at, id) < ($1, $2)
				 ORDER BY created_at DESC, id DESC
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("querying config versions: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var v ConfigVersion
			if scanErr := rows.Scan(&v.ID, &v.OrganizationID, &v.Version, &v.Snapshot, &v.Author, &v.Description, &v.CreatedAt); scanErr != nil {
				return fmt.Errorf("scanning config version: %w", scanErr)
			}
			versions = append(versions, v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing config versions: %w", err)
	}

	return buildPagedResponse(versions, limit, func(v ConfigVersion) (time.Time, uuid.UUID) {
		return v.CreatedAt, v.ID
	}), nil
}

// GetVersion returns a single config version by org and version number.
func (s *Store) GetVersion(ctx context.Context, version int) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting config version: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var cv ConfigVersion
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, version, snapshot, author, description, created_at
			 FROM config_versions
			 WHERE version = $1
			 LIMIT 1`,
			version,
		).Scan(&cv.ID, &cv.OrganizationID, &cv.Version, &cv.Snapshot, &cv.Author, &cv.Description, &cv.CreatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("config version %d: %w", version, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("getting config version %d: %w", version, err)
	}

	return &cv, nil
}

// GetLatestVersion returns the most recent config version for an org.
func (s *Store) GetLatestVersion(ctx context.Context) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting latest config version: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var cv ConfigVersion
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, version, snapshot, author, description, created_at
			 FROM config_versions
			 ORDER BY version DESC
			 LIMIT 1`,
		).Scan(&cv.ID, &cv.OrganizationID, &cv.Version, &cv.Snapshot, &cv.Author, &cv.Description, &cv.CreatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no config versions: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("getting latest config version: %w", err)
	}

	return &cv, nil
}

// RollbackToVersion reads the snapshot from the target version and creates a new version
// with that snapshot. This is append-only -- no history is deleted.
func (s *Store) RollbackToVersion(ctx context.Context, targetVersion int) (*ConfigVersion, error) {
	target, err := s.GetVersion(ctx, targetVersion)
	if err != nil {
		return nil, err
	}

	author := fmt.Sprintf("rollback:v%d", targetVersion)
	desc := fmt.Sprintf("rollback to version %d", targetVersion)
	return s.CreateVersion(ctx, target.Snapshot, author, desc)
}
