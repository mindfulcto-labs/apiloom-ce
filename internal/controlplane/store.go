package controlplane

import (
	"context"
	"encoding/base64"
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

// Store provides control plane database operations with update methods,
// cursor-based pagination, and AI model/token budget CRUD.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new control plane Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// --- Cursor encoding/decoding ---

// encodeCursor produces a base64-encoded cursor from a timestamp and UUID.
func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := t.Format(time.RFC3339Nano) + "|" + id.String()
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses a base64-encoded cursor back into timestamp and UUID.
// Returns zero values for empty cursor (first page).
func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	if cursor == "" {
		return time.Time{}, uuid.Nil, nil
	}

	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("decoding cursor: %w", err)
	}

	parts := strings.SplitN(string(raw), "|", 2)
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

// encodeTestCursor is a test helper that creates a raw base64 cursor from string parts.
func encodeTestCursor(timeStr, idStr string) string {
	raw := timeStr + "|" + idStr
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// buildPagedResponse constructs a PagedResponse from a result slice that may contain
// limit+1 items. The cursorFn extracts (created_at, id) from the last visible item
// to produce the next_cursor.
func buildPagedResponse[T any](items []T, limit int, cursorFn func(T) (time.Time, uuid.UUID)) *PagedResponse[T] {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	resp := &PagedResponse[T]{
		Data:    items,
		HasMore: hasMore,
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		t, id := cursorFn(last)
		resp.NextCursor = encodeCursor(t, id)
	}

	return resp
}

// --- Routes ---

// UpdateRoute updates a route by ID with PUT semantics (full replacement of mutable fields).
// Bumps version and recomputes checksum. Returns domain.ErrNotFound if route doesn't exist.
func (s *Store) UpdateRoute(ctx context.Context, id uuid.UUID, req UpdateRouteRequest) (*domain.RouteConfig, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating route: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	checksum := domain.ComputeChecksum(req.Config)

	var r domain.RouteConfig
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE routes
			 SET name = $1, config = $2, metadata = $3, version = version + 1,
			     checksum = $4, updated_at = NOW()
			 WHERE id = $5 AND is_active = true
			 RETURNING id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at`,
			req.Name, req.Config, req.Metadata, checksum, id,
		).Scan(&r.ID, &r.OrganizationID, &r.Name, &r.Version, &r.Config, &r.Metadata, &r.Checksum, &r.IsActive, &r.CreatedAt, &r.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating route %s: %w", id, err)
	}

	return &r, nil
}

// ListRoutesPaged returns paginated active routes ordered by (created_at, id).
func (s *Store) ListRoutesPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.RouteConfig], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var routes []domain.RouteConfig
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at
				 FROM routes WHERE is_active = true
				 ORDER BY created_at, id
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, version, config, metadata, checksum, is_active, created_at, updated_at
				 FROM routes
				 WHERE is_active = true AND (created_at, id) > ($1, $2)
				 ORDER BY created_at, id
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing routes: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.RouteConfig
			if scanErr := rows.Scan(&r.ID, &r.OrganizationID, &r.Name, &r.Version, &r.Config, &r.Metadata, &r.Checksum, &r.IsActive, &r.CreatedAt, &r.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning route: %w", scanErr)
			}
			routes = append(routes, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}

	return buildPagedResponse(routes, limit, func(r domain.RouteConfig) (time.Time, uuid.UUID) {
		return r.CreatedAt, r.ID
	}), nil
}

// --- Backends ---

// UpdateBackend updates a backend by ID with PUT semantics.
func (s *Store) UpdateBackend(ctx context.Context, id uuid.UUID, req UpdateBackendRequest) (*domain.BackendConfig, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating backend: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var b domain.BackendConfig
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE backends
			 SET name = $1, config = $2, metadata = $3, updated_at = NOW()
			 WHERE id = $4
			 RETURNING id, organization_id, name, config, metadata, created_at, updated_at`,
			req.Name, req.Config, req.Metadata, id,
		).Scan(&b.ID, &b.OrganizationID, &b.Name, &b.Config, &b.Metadata, &b.CreatedAt, &b.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating backend %s: %w", id, err)
	}

	return &b, nil
}

// ListBackendsPaged returns paginated backends ordered by (created_at, id).
func (s *Store) ListBackendsPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.BackendConfig], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var backends []domain.BackendConfig
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, config, metadata, created_at, updated_at
				 FROM backends
				 ORDER BY created_at, id
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, config, metadata, created_at, updated_at
				 FROM backends
				 WHERE (created_at, id) > ($1, $2)
				 ORDER BY created_at, id
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing backends: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var b domain.BackendConfig
			if scanErr := rows.Scan(&b.ID, &b.OrganizationID, &b.Name, &b.Config, &b.Metadata, &b.CreatedAt, &b.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning backend: %w", scanErr)
			}
			backends = append(backends, b)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}

	return buildPagedResponse(backends, limit, func(b domain.BackendConfig) (time.Time, uuid.UUID) {
		return b.CreatedAt, b.ID
	}), nil
}

// --- Consumers ---

// UpdateConsumer updates a consumer by ID with PUT semantics.
func (s *Store) UpdateConsumer(ctx context.Context, id uuid.UUID, req UpdateConsumerRequest) (*domain.Consumer, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating consumer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var c domain.Consumer
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE consumers
			 SET name = $1, metadata = $2, updated_at = NOW()
			 WHERE id = $3 AND is_active = true
			 RETURNING id, organization_id, name, metadata, is_active, created_at, updated_at`,
			req.Name, req.Metadata, id,
		).Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Metadata, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("consumer %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating consumer %s: %w", id, err)
	}

	return &c, nil
}

// ListConsumersPaged returns paginated active consumers ordered by (created_at, id).
func (s *Store) ListConsumersPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.Consumer], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing consumers: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var consumers []domain.Consumer
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, metadata, is_active, created_at, updated_at
				 FROM consumers WHERE is_active = true
				 ORDER BY created_at, id
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, name, metadata, is_active, created_at, updated_at
				 FROM consumers
				 WHERE is_active = true AND (created_at, id) > ($1, $2)
				 ORDER BY created_at, id
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing consumers: %w", qErr)
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

	return buildPagedResponse(consumers, limit, func(c domain.Consumer) (time.Time, uuid.UUID) {
		return c.CreatedAt, c.ID
	}), nil
}

// --- API Keys ---

// ListAPIKeys returns paginated API keys for a consumer.
func (s *Store) ListAPIKeys(ctx context.Context, consumerID uuid.UUID, params ListParams) (*PagedResponse[domain.APIKey], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var keys []domain.APIKey
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, created_at
				 FROM api_keys WHERE consumer_id = $1
				 ORDER BY created_at, id
				 LIMIT $2`,
				consumerID, limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, consumer_id, key_hash, key_prefix, is_active, deprecated_at, expires_at, last_used_at, rate_limit_override, created_at
				 FROM api_keys
				 WHERE consumer_id = $1 AND (created_at, id) > ($2, $3)
				 ORDER BY created_at, id
				 LIMIT $4`,
				consumerID, cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing API keys: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var k domain.APIKey
			if scanErr := rows.Scan(&k.ID, &k.OrganizationID, &k.ConsumerID, &k.KeyHash, &k.KeyPrefix, &k.IsActive,
				&k.DeprecatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RateLimitOverride, &k.CreatedAt); scanErr != nil {
				return fmt.Errorf("scanning API key: %w", scanErr)
			}
			keys = append(keys, k)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}

	return buildPagedResponse(keys, limit, func(k domain.APIKey) (time.Time, uuid.UUID) {
		return k.CreatedAt, k.ID
	}), nil
}

// --- AI Models ---

// AIModelRow represents an AI model row from the database.
type AIModelRow struct {
	ID                  uuid.UUID `json:"id"`
	OrganizationID      uuid.UUID `json:"organization_id"`
	Provider            string    `json:"provider"`
	ModelName           string    `json:"model_name"`
	DisplayName         string    `json:"display_name,omitempty"`
	InputPricePerToken  float64   `json:"input_price_per_token"`
	OutputPricePerToken float64   `json:"output_price_per_token"`
	Capabilities        []string  `json:"capabilities"`
	IsActive            bool      `json:"is_active"`
	Weight              int       `json:"weight"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreateAIModel inserts a new AI model and returns the saved entity.
func (s *Store) CreateAIModel(ctx context.Context, req CreateAIModelRequest) (*AIModelRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating AI model: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	weight := req.Weight
	if weight == 0 {
		weight = 1
	}

	var m AIModelRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO ai_models (organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, weight)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id, organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, created_at, updated_at`,
			orgID, req.Provider, req.ModelName, req.DisplayName,
			req.InputPricePerToken, req.OutputPricePerToken,
			req.Capabilities, weight,
		).Scan(&m.ID, &m.OrganizationID, &m.Provider, &m.ModelName, &m.DisplayName,
			&m.InputPricePerToken, &m.OutputPricePerToken,
			&m.Capabilities, &m.IsActive, &m.Weight,
			&m.CreatedAt, &m.UpdatedAt)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("ai model %s/%s: %w", req.Provider, req.ModelName, domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting ai model: %w", err)
	}

	return &m, nil
}

// GetAIModel returns a single AI model by ID.
func (s *Store) GetAIModel(ctx context.Context, id uuid.UUID) (*AIModelRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting AI model: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var m AIModelRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, created_at, updated_at
			 FROM ai_models WHERE id = $1`,
			id,
		).Scan(&m.ID, &m.OrganizationID, &m.Provider, &m.ModelName, &m.DisplayName,
			&m.InputPricePerToken, &m.OutputPricePerToken,
			&m.Capabilities, &m.IsActive, &m.Weight,
			&m.CreatedAt, &m.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading ai model %s: %w", id, err)
	}

	return &m, nil
}

// ListAIModelsPaged returns paginated active AI models.
func (s *Store) ListAIModelsPaged(ctx context.Context, params ListParams) (*PagedResponse[AIModelRow], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing AI models: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var models []AIModelRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, created_at, updated_at
				 FROM ai_models WHERE is_active = true
				 ORDER BY created_at, id
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, created_at, updated_at
				 FROM ai_models
				 WHERE is_active = true AND (created_at, id) > ($1, $2)
				 ORDER BY created_at, id
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing ai models: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var m AIModelRow
			if scanErr := rows.Scan(&m.ID, &m.OrganizationID, &m.Provider, &m.ModelName, &m.DisplayName,
				&m.InputPricePerToken, &m.OutputPricePerToken,
				&m.Capabilities, &m.IsActive, &m.Weight,
				&m.CreatedAt, &m.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning ai model: %w", scanErr)
			}
			models = append(models, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing ai models: %w", err)
	}

	return buildPagedResponse(models, limit, func(m AIModelRow) (time.Time, uuid.UUID) {
		return m.CreatedAt, m.ID
	}), nil
}

// UpdateAIModel updates an AI model by ID.
func (s *Store) UpdateAIModel(ctx context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating AI model: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var m AIModelRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE ai_models
			 SET provider = $1, model_name = $2, display_name = $3,
			     input_price_per_token = $4, output_price_per_token = $5,
			     capabilities = $6, weight = $7, updated_at = NOW()
			 WHERE id = $8 AND is_active = true
			 RETURNING id, organization_id, provider, model_name, display_name, input_price_per_token, output_price_per_token, capabilities, is_active, weight, created_at, updated_at`,
			req.Provider, req.ModelName, req.DisplayName,
			req.InputPricePerToken, req.OutputPricePerToken,
			req.Capabilities, req.Weight, id,
		).Scan(&m.ID, &m.OrganizationID, &m.Provider, &m.ModelName, &m.DisplayName,
			&m.InputPricePerToken, &m.OutputPricePerToken,
			&m.Capabilities, &m.IsActive, &m.Weight,
			&m.CreatedAt, &m.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating ai model %s: %w", id, err)
	}

	return &m, nil
}

// DeleteAIModel soft-deletes an AI model by setting is_active=false.
func (s *Store) DeleteAIModel(ctx context.Context, id uuid.UUID) error {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting AI model: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`UPDATE ai_models SET is_active = false, updated_at = NOW() WHERE id = $1 AND is_active = true`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting ai model %s: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// --- Token Budgets ---

// TokenBudgetRow represents a token budget row from the database.
type TokenBudgetRow struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ConsumerID     *uuid.UUID `json:"consumer_id,omitempty"`
	Team           *string    `json:"team,omitempty"`
	Period         string     `json:"period"`
	MaxTokens      *int64     `json:"max_tokens,omitempty"`
	MaxCostUSD     *float64   `json:"max_cost_usd,omitempty"`
	CurrentTokens  int64      `json:"current_tokens"`
	CurrentCostUSD float64    `json:"current_cost_usd"`
	PeriodStart    time.Time  `json:"period_start"`
	Enforcement    string     `json:"enforcement"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CreateTokenBudget inserts a new token budget and returns the saved entity.
func (s *Store) CreateTokenBudget(ctx context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating token budget: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	enforcement := req.Enforcement
	if enforcement == "" {
		enforcement = "hard"
	}

	var tb TokenBudgetRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO token_budgets (organization_id, consumer_id, team, period, max_tokens, max_cost_usd, enforcement)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id, organization_id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, created_at, updated_at`,
			orgID, req.ConsumerID, nullIfEmpty(req.Team), req.Period, req.MaxTokens, req.MaxCostUSD, enforcement,
		).Scan(&tb.ID, &tb.OrganizationID, &tb.ConsumerID, &tb.Team, &tb.Period, &tb.MaxTokens, &tb.MaxCostUSD,
			&tb.CurrentTokens, &tb.CurrentCostUSD, &tb.PeriodStart, &tb.Enforcement,
			&tb.CreatedAt, &tb.UpdatedAt)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("token budget: %w", domain.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("inserting token budget: %w", err)
	}

	return &tb, nil
}

// GetTokenBudget returns a single token budget by ID.
func (s *Store) GetTokenBudget(ctx context.Context, id uuid.UUID) (*TokenBudgetRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting token budget: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var tb TokenBudgetRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, organization_id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, created_at, updated_at
			 FROM token_budgets WHERE id = $1`,
			id,
		).Scan(&tb.ID, &tb.OrganizationID, &tb.ConsumerID, &tb.Team, &tb.Period, &tb.MaxTokens, &tb.MaxCostUSD,
			&tb.CurrentTokens, &tb.CurrentCostUSD, &tb.PeriodStart, &tb.Enforcement,
			&tb.CreatedAt, &tb.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("token budget %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("loading token budget %s: %w", id, err)
	}

	return &tb, nil
}

// ListTokenBudgetsPaged returns paginated token budgets.
func (s *Store) ListTokenBudgetsPaged(ctx context.Context, params ListParams) (*PagedResponse[TokenBudgetRow], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing token budgets: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := params.EffectiveLimit()
	cursorTime, cursorID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var budgets []TokenBudgetRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		var rows pgx.Rows
		var qErr error
		if params.Cursor == "" {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, created_at, updated_at
				 FROM token_budgets
				 ORDER BY created_at, id
				 LIMIT $1`,
				limit+1,
			)
		} else {
			rows, qErr = tx.Query(ctx,
				`SELECT id, organization_id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, created_at, updated_at
				 FROM token_budgets
				 WHERE (created_at, id) > ($1, $2)
				 ORDER BY created_at, id
				 LIMIT $3`,
				cursorTime, cursorID, limit+1,
			)
		}
		if qErr != nil {
			return fmt.Errorf("listing token budgets: %w", qErr)
		}
		defer rows.Close()

		for rows.Next() {
			var tb TokenBudgetRow
			if scanErr := rows.Scan(&tb.ID, &tb.OrganizationID, &tb.ConsumerID, &tb.Team, &tb.Period, &tb.MaxTokens, &tb.MaxCostUSD,
				&tb.CurrentTokens, &tb.CurrentCostUSD, &tb.PeriodStart, &tb.Enforcement,
				&tb.CreatedAt, &tb.UpdatedAt); scanErr != nil {
				return fmt.Errorf("scanning token budget: %w", scanErr)
			}
			budgets = append(budgets, tb)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing token budgets: %w", err)
	}

	return buildPagedResponse(budgets, limit, func(tb TokenBudgetRow) (time.Time, uuid.UUID) {
		return tb.CreatedAt, tb.ID
	}), nil
}

// UpdateTokenBudget updates a token budget by ID.
func (s *Store) UpdateTokenBudget(ctx context.Context, id uuid.UUID, req UpdateTokenBudgetRequest) (*TokenBudgetRow, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating token budget: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	enforcement := req.Enforcement
	if enforcement == "" {
		enforcement = "hard"
	}

	var tb TokenBudgetRow
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE token_budgets
			 SET consumer_id = $1, team = $2, period = $3, max_tokens = $4, max_cost_usd = $5,
			     enforcement = $6, updated_at = NOW()
			 WHERE id = $7
			 RETURNING id, organization_id, consumer_id, team, period, max_tokens, max_cost_usd, current_tokens, current_cost_usd, period_start, enforcement, created_at, updated_at`,
			req.ConsumerID, nullIfEmpty(req.Team), req.Period, req.MaxTokens, req.MaxCostUSD, enforcement, id,
		).Scan(&tb.ID, &tb.OrganizationID, &tb.ConsumerID, &tb.Team, &tb.Period, &tb.MaxTokens, &tb.MaxCostUSD,
			&tb.CurrentTokens, &tb.CurrentCostUSD, &tb.PeriodStart, &tb.Enforcement,
			&tb.CreatedAt, &tb.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("token budget %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("updating token budget %s: %w", id, err)
	}

	return &tb, nil
}

// DeleteTokenBudget hard-deletes a token budget by ID.
func (s *Store) DeleteTokenBudget(ctx context.Context, id uuid.UUID) error {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return fmt.Errorf("deleting token budget: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rowsAffected int64
	err = config.WithOrgTx(ctx, s.pool, orgID, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx,
			`DELETE FROM token_budgets WHERE id = $1`,
			id,
		)
		if execErr != nil {
			return execErr
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return fmt.Errorf("deleting token budget %s: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("token budget %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// nullIfEmpty returns nil for empty strings (for nullable TEXT columns).
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
