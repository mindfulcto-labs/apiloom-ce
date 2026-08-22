package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// PluginRecord represents a plugin row from the database.
type PluginRecord struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Name           string          `json:"name"`
	PluginType     string          `json:"plugin_type"`
	Config         json.RawMessage `json:"config"`
	Target         string          `json:"target"`
	GRPCAddress    *string         `json:"grpc_address,omitempty"`
	Ordering       int             `json:"ordering"`
	IsEnabled      bool            `json:"is_enabled"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ListPluginParams holds filter and pagination parameters for listing plugins.
type ListPluginParams struct {
	Cursor     string
	Limit      int
	IsEnabled  *bool
	PluginType string
}

// EffectiveLimit returns the limit capped between 1 and 100, defaulting to 20.
func (p ListPluginParams) EffectiveLimit() int {
	if p.Limit <= 0 {
		return 20
	}
	if p.Limit > 100 {
		return 100
	}
	return p.Limit
}

// PagedPlugins wraps a plugin slice with cursor pagination metadata.
type PagedPlugins struct {
	Data       []PluginRecord `json:"data"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// PluginStore defines the interface for plugin persistence operations.
// Defined in consumer package (plugin) per interfaces-in-consumer rule.
type PluginStore interface {
	Create(ctx context.Context, rec *PluginRecord) (*PluginRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*PluginRecord, error)
	List(ctx context.Context, params ListPluginParams) (*PagedPlugins, error)
	UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) (*PluginRecord, error)
	UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*PluginRecord, error)
	UpdateOrdering(ctx context.Context, id uuid.UUID, ordering int) (*PluginRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// PostgresStore implements PluginStore backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Create inserts a new plugin record and returns it with generated fields.
func (s *PostgresStore) Create(ctx context.Context, rec *PluginRecord) (*PluginRecord, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating plugin: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO plugins (organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at`,
			orgID, rec.Name, rec.PluginType, rec.Config, rec.Target, rec.GRPCAddress, rec.Ordering, rec.IsEnabled,
		).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("plugin %s: %w", rec.Name, domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting plugin: %w", err)
	}

	return &p, nil
}

// Get returns a single plugin by ID.
func (s *PostgresStore) Get(ctx context.Context, id uuid.UUID) (*PluginRecord, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting plugin: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at
			 FROM plugins WHERE id = $1`,
			id,
		).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading plugin %s: %w", id, err)
	}

	return &p, nil
}

// List returns paginated plugins with optional filters.
func (s *PostgresStore) List(ctx context.Context, params ListPluginParams) (*PagedPlugins, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing plugins: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var plugins []PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		// Build dynamic query with filters.
		query := `SELECT id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at FROM plugins WHERE 1=1`
		args := []any{}
		argIdx := 1

		if params.IsEnabled != nil {
			query += fmt.Sprintf(" AND is_enabled = $%d", argIdx)
			args = append(args, *params.IsEnabled)
			argIdx++
		}
		if params.PluginType != "" {
			query += fmt.Sprintf(" AND plugin_type = $%d", argIdx)
			args = append(args, params.PluginType)
			argIdx++
		}
		if params.Cursor != "" {
			query += fmt.Sprintf(" AND (created_at, id) > ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, cursorTime, cursorID)
			argIdx += 2
		}

		query += fmt.Sprintf(" ORDER BY created_at, id LIMIT $%d", argIdx)
		args = append(args, limit+1)

		rows, qErr := tx.Query(ctx, query, args...)
		if qErr != nil {
			return fmt.Errorf("listing plugins: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var p PluginRecord
			if scanErr := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning plugin: %w", scanErr)
			}
			plugins = append(plugins, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing plugins: %w", err)
	}

	hasMore := len(plugins) > limit
	if hasMore {
		plugins = plugins[:limit]
	}

	resp := &PagedPlugins{
		Data:    plugins,
		HasMore: hasMore,
	}
	if hasMore && len(plugins) > 0 {
		last := plugins[len(plugins)-1]
		resp.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return resp, nil
}

// UpdateConfig updates a plugin's config JSON.
func (s *PostgresStore) UpdateConfig(ctx context.Context, id uuid.UUID, cfg json.RawMessage) (*PluginRecord, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating plugin config: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE plugins SET config = $1, updated_at = NOW()
			 WHERE id = $2
			 RETURNING id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at`,
			cfg, id,
		).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating plugin config %s: %w", id, err)
	}

	return &p, nil
}

// UpdateEnabled sets the is_enabled flag for a plugin.
func (s *PostgresStore) UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*PluginRecord, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating plugin enabled: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE plugins SET is_enabled = $1, updated_at = NOW()
			 WHERE id = $2
			 RETURNING id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at`,
			enabled, id,
		).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating plugin enabled %s: %w", id, err)
	}

	return &p, nil
}

// UpdateOrdering sets the ordering integer for a plugin.
func (s *PostgresStore) UpdateOrdering(ctx context.Context, id uuid.UUID, ordering int) (*PluginRecord, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating plugin ordering: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p PluginRecord
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE plugins SET ordering = $1, updated_at = NOW()
			 WHERE id = $2
			 RETURNING id, organization_id, name, plugin_type, config, target, grpc_address, ordering, is_enabled, created_at, updated_at`,
			ordering, id,
		).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.PluginType, &p.Config, &p.Target, &p.GRPCAddress, &p.Ordering, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating plugin ordering %s: %w", id, err)
	}

	return &p, nil
}

// Delete hard-deletes a plugin by ID.
func (s *PostgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting plugin: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`DELETE FROM plugins WHERE id = $1`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting plugin %s: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// --- Cursor helpers (same pattern as controlplane/store.go) ---

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := t.Format(time.RFC3339Nano) + "|" + id.String()
	return encodeBase64(raw)
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	if cursor == "" {
		return time.Time{}, uuid.Nil, nil
	}

	raw, err := decodeBase64(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("decoding cursor: %w", err)
	}

	parts := splitCursor(raw)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}

	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("parsing cursor time: %w", err)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("parsing cursor id: %w", err)
	}

	return t, id, nil
}

func encodeBase64(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func decodeBase64(s string) (string, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func splitCursor(raw string) []string {
	return strings.SplitN(raw, "|", 2)
}
