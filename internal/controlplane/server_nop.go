package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/cto-venkat/apiloom-ce/internal/plugin"
)

// poolStatusDB implements StatusDB using a pgxpool.Pool.
type poolStatusDB struct {
	pool *pgxpool.Pool
}

func (db *poolStatusDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

func (db *poolStatusDB) CountActive(ctx context.Context, table string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Only allow known tables to prevent SQL injection.
	var query string
	switch table {
	case "routes":
		query = `SELECT COUNT(*) FROM routes WHERE is_active = true`
	case "backends":
		query = `SELECT COUNT(*) FROM backends`
	case "consumers":
		query = `SELECT COUNT(*) FROM consumers WHERE is_active = true`
	default:
		return 0, fmt.Errorf("unknown table: %s", table)
	}

	var count int64
	err := db.pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

// nopStatusDB is a no-op StatusDB for testing without a database.
type nopStatusDB struct{}

func (db *nopStatusDB) Ping(_ context.Context) error                           { return nil }
func (db *nopStatusDB) CountActive(_ context.Context, _ string) (int64, error) { return 0, nil }

// nopRouteConfigStore is a no-op implementation for testing.
type nopRouteConfigStore struct{}

func (s *nopRouteConfigStore) SaveRoute(_ context.Context, _ *domain.RouteConfig) (*domain.RouteConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopRouteConfigStore) LoadRoute(_ context.Context, _ uuid.UUID) (*domain.RouteConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopRouteConfigStore) DeleteRoute(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}

// nopRouteStore is a no-op implementation for testing.
type nopRouteStore struct{}

func (s *nopRouteStore) UpdateRoute(_ context.Context, _ uuid.UUID, _ UpdateRouteRequest) (*domain.RouteConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopRouteStore) ListRoutesPaged(_ context.Context, _ ListParams) (*PagedResponse[domain.RouteConfig], error) {
	return &PagedResponse[domain.RouteConfig]{Data: []domain.RouteConfig{}}, nil
}

// nopBackendConfigStore is a no-op implementation for testing.
type nopBackendConfigStore struct{}

func (s *nopBackendConfigStore) SaveBackend(_ context.Context, _ *domain.BackendConfig) (*domain.BackendConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopBackendConfigStore) LoadBackend(_ context.Context, _ uuid.UUID) (*domain.BackendConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopBackendConfigStore) DeleteBackend(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}

// nopBackendStore is a no-op implementation for testing.
type nopBackendStore struct{}

func (s *nopBackendStore) UpdateBackend(_ context.Context, _ uuid.UUID, _ UpdateBackendRequest) (*domain.BackendConfig, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopBackendStore) ListBackendsPaged(_ context.Context, _ ListParams) (*PagedResponse[domain.BackendConfig], error) {
	return &PagedResponse[domain.BackendConfig]{Data: []domain.BackendConfig{}}, nil
}

// nopConsumerConfigStore is a no-op implementation for testing.
type nopConsumerConfigStore struct{}

func (s *nopConsumerConfigStore) CreateConsumer(_ context.Context, _ string, _ json.RawMessage) (*domain.Consumer, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) GetConsumer(_ context.Context, _ uuid.UUID) (*domain.Consumer, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) DeleteConsumer(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) CreateAPIKey(_ context.Context, _ uuid.UUID, _ json.RawMessage) (string, *domain.APIKey, error) {
	return "", nil, fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) RevokeAPIKey(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) RotateAPIKey(_ context.Context, _ uuid.UUID, _ time.Duration) (string, *domain.APIKey, error) {
	return "", nil, fmt.Errorf("no database connection")
}
func (s *nopConsumerConfigStore) GetAPIKey(_ context.Context, _ uuid.UUID) (*domain.APIKey, error) {
	return nil, fmt.Errorf("no database connection")
}

// nopConsumerStore is a no-op implementation for testing.
type nopConsumerStore struct{}

func (s *nopConsumerStore) UpdateConsumer(_ context.Context, _ uuid.UUID, _ UpdateConsumerRequest) (*domain.Consumer, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConsumerStore) ListConsumersPaged(_ context.Context, _ ListParams) (*PagedResponse[domain.Consumer], error) {
	return &PagedResponse[domain.Consumer]{Data: []domain.Consumer{}}, nil
}
func (s *nopConsumerStore) ListAPIKeys(_ context.Context, _ uuid.UUID, _ ListParams) (*PagedResponse[domain.APIKey], error) {
	return &PagedResponse[domain.APIKey]{Data: []domain.APIKey{}}, nil
}

// nopAIStore is a no-op implementation for testing.
type nopAIStore struct{}

func (s *nopAIStore) CreateAIModel(_ context.Context, _ CreateAIModelRequest) (*AIModelRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) ListAIModelsPaged(_ context.Context, _ ListParams) (*PagedResponse[AIModelRow], error) {
	return &PagedResponse[AIModelRow]{Data: []AIModelRow{}}, nil
}
func (s *nopAIStore) GetAIModel(_ context.Context, _ uuid.UUID) (*AIModelRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) UpdateAIModel(_ context.Context, _ uuid.UUID, _ UpdateAIModelRequest) (*AIModelRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) DeleteAIModel(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}
func (s *nopAIStore) CreateTokenBudget(_ context.Context, _ CreateTokenBudgetRequest) (*TokenBudgetRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) ListTokenBudgetsPaged(_ context.Context, _ ListParams) (*PagedResponse[TokenBudgetRow], error) {
	return &PagedResponse[TokenBudgetRow]{Data: []TokenBudgetRow{}}, nil
}
func (s *nopAIStore) GetTokenBudget(_ context.Context, _ uuid.UUID) (*TokenBudgetRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) UpdateTokenBudget(_ context.Context, _ uuid.UUID, _ UpdateTokenBudgetRequest) (*TokenBudgetRow, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopAIStore) DeleteTokenBudget(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}

// nopConfigVersionStore is a no-op implementation for testing.
type nopConfigVersionStore struct{}

func (s *nopConfigVersionStore) CreateVersion(_ context.Context, _ json.RawMessage, _, _ string) (*ConfigVersion, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConfigVersionStore) ListVersions(_ context.Context, _ ListParams) (*PagedResponse[ConfigVersion], error) {
	return &PagedResponse[ConfigVersion]{Data: []ConfigVersion{}}, nil
}
func (s *nopConfigVersionStore) GetVersion(_ context.Context, _ int) (*ConfigVersion, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConfigVersionStore) GetLatestVersion(_ context.Context) (*ConfigVersion, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopConfigVersionStore) RollbackToVersion(_ context.Context, _ int) (*ConfigVersion, error) {
	return nil, fmt.Errorf("no database connection")
}

// nopPluginStore is a no-op implementation for testing.
type nopPluginStore struct{}

func (s *nopPluginStore) Create(_ context.Context, _ *plugin.PluginRecord) (*plugin.PluginRecord, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopPluginStore) Get(_ context.Context, _ uuid.UUID) (*plugin.PluginRecord, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopPluginStore) List(_ context.Context, _ plugin.ListPluginParams) (*plugin.PagedPlugins, error) {
	return &plugin.PagedPlugins{Data: []plugin.PluginRecord{}}, nil
}
func (s *nopPluginStore) UpdateConfig(_ context.Context, _ uuid.UUID, _ json.RawMessage) (*plugin.PluginRecord, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopPluginStore) UpdateEnabled(_ context.Context, _ uuid.UUID, _ bool) (*plugin.PluginRecord, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopPluginStore) UpdateOrdering(_ context.Context, _ uuid.UUID, _ int) (*plugin.PluginRecord, error) {
	return nil, fmt.Errorf("no database connection")
}
func (s *nopPluginStore) Delete(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("no database connection")
}

