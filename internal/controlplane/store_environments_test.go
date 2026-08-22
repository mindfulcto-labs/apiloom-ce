package controlplane

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// mockEnvironmentStore implements EnvironmentStore for handler-level unit tests.
type mockEnvironmentStore struct {
	listFn    func(ctx context.Context, orgID uuid.UUID) ([]domain.Environment, error)
	getFn     func(ctx context.Context, orgID, envID uuid.UUID) (*domain.Environment, error)
	createFn  func(ctx context.Context, env *domain.Environment) error
	deleteFn  func(ctx context.Context, orgID, envID uuid.UUID) error
	promoteFn func(ctx context.Context, orgID, src, tgt uuid.UUID) (*domain.PromotionResult, error)
	countFn   func(ctx context.Context, orgID uuid.UUID) (int, error)
}

func (m *mockEnvironmentStore) ListEnvironments(ctx context.Context, orgID uuid.UUID) ([]domain.Environment, error) {
	if m.listFn != nil {
		return m.listFn(ctx, orgID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEnvironmentStore) GetEnvironment(ctx context.Context, orgID, envID uuid.UUID) (*domain.Environment, error) {
	if m.getFn != nil {
		return m.getFn(ctx, orgID, envID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEnvironmentStore) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	if m.createFn != nil {
		return m.createFn(ctx, env)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockEnvironmentStore) DeleteEnvironment(ctx context.Context, orgID, envID uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, orgID, envID)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockEnvironmentStore) PromoteEnvironment(ctx context.Context, orgID, src, tgt uuid.UUID) (*domain.PromotionResult, error) {
	if m.promoteFn != nil {
		return m.promoteFn(ctx, orgID, src, tgt)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEnvironmentStore) CountEnvironments(ctx context.Context, orgID uuid.UUID) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, orgID)
	}
	return 0, fmt.Errorf("not implemented")
}

func TestEnvironmentStore_NopImplementation(t *testing.T) {
	t.Parallel()

	store := &nopEnvironmentStore{}
	ctx := context.Background()
	orgID := uuid.New()
	envID := uuid.New()

	envs, err := store.ListEnvironments(ctx, orgID)
	require.NoError(t, err)
	assert.Empty(t, envs)

	_, err = store.GetEnvironment(ctx, orgID, envID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	err = store.CreateEnvironment(ctx, &domain.Environment{})
	require.NoError(t, err)

	err = store.DeleteEnvironment(ctx, orgID, envID)
	require.NoError(t, err)

	result, err := store.PromoteEnvironment(ctx, orgID, uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.NotNil(t, result)

	count, err := store.CountEnvironments(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestEnvironmentStore_MockCRUD(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	now := time.Now().UTC()

	devEnv := domain.Environment{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Name:           "Development",
		Slug:           "development",
		Description:    "Dev environment",
		IsProduction:   false,
		SortOrder:      0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	prodEnv := domain.Environment{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Name:           "Production",
		Slug:           "production",
		Description:    "Prod environment",
		IsProduction:   true,
		SortOrder:      2,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	store := &mockEnvironmentStore{
		listFn: func(_ context.Context, id uuid.UUID) ([]domain.Environment, error) {
			if id == orgID {
				return []domain.Environment{devEnv, prodEnv}, nil
			}
			return []domain.Environment{}, nil
		},
		getFn: func(_ context.Context, oID, eID uuid.UUID) (*domain.Environment, error) {
			if oID == orgID && eID == devEnv.ID {
				return &devEnv, nil
			}
			if oID == orgID && eID == prodEnv.ID {
				return &prodEnv, nil
			}
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, env *domain.Environment) error {
			if env.Name == "" {
				return domain.ErrInvalidConfig
			}
			return nil
		},
		deleteFn: func(_ context.Context, oID, eID uuid.UUID) error {
			if oID == orgID && eID == prodEnv.ID {
				return fmt.Errorf("cannot delete production environment: %w", domain.ErrForbidden)
			}
			return nil
		},
		promoteFn: func(_ context.Context, _ uuid.UUID, src, tgt uuid.UUID) (*domain.PromotionResult, error) {
			if src == tgt {
				return nil, domain.ErrInvalidConfig
			}
			return &domain.PromotionResult{RoutesCreated: 2, BackendsCreated: 1}, nil
		},
		countFn: func(_ context.Context, id uuid.UUID) (int, error) {
			if id == orgID {
				return 2, nil
			}
			return 0, nil
		},
	}

	ctx := context.Background()

	t.Run("list environments", func(t *testing.T) {
		t.Parallel()
		envs, err := store.ListEnvironments(ctx, orgID)
		require.NoError(t, err)
		assert.Len(t, envs, 2)
		assert.Equal(t, "Development", envs[0].Name)
		assert.Equal(t, "Production", envs[1].Name)
	})

	t.Run("list returns empty for unknown org", func(t *testing.T) {
		t.Parallel()
		envs, err := store.ListEnvironments(ctx, uuid.New())
		require.NoError(t, err)
		assert.Empty(t, envs)
	})

	t.Run("get environment", func(t *testing.T) {
		t.Parallel()
		env, err := store.GetEnvironment(ctx, orgID, devEnv.ID)
		require.NoError(t, err)
		assert.Equal(t, "Development", env.Name)
	})

	t.Run("get environment not found", func(t *testing.T) {
		t.Parallel()
		_, err := store.GetEnvironment(ctx, orgID, uuid.New())
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("create environment", func(t *testing.T) {
		t.Parallel()
		env := &domain.Environment{Name: "Test"}
		err := store.CreateEnvironment(ctx, env)
		require.NoError(t, err)
	})

	t.Run("delete non-production", func(t *testing.T) {
		t.Parallel()
		err := store.DeleteEnvironment(ctx, orgID, devEnv.ID)
		require.NoError(t, err)
	})

	t.Run("delete production fails", func(t *testing.T) {
		t.Parallel()
		err := store.DeleteEnvironment(ctx, orgID, prodEnv.ID)
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("promote environment", func(t *testing.T) {
		t.Parallel()
		result, err := store.PromoteEnvironment(ctx, orgID, devEnv.ID, prodEnv.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, result.RoutesCreated)
		assert.Equal(t, 1, result.BackendsCreated)
	})

	t.Run("promote same env fails", func(t *testing.T) {
		t.Parallel()
		_, err := store.PromoteEnvironment(ctx, orgID, devEnv.ID, devEnv.ID)
		assert.ErrorIs(t, err, domain.ErrInvalidConfig)
	})

	t.Run("count environments", func(t *testing.T) {
		t.Parallel()
		count, err := store.CountEnvironments(ctx, orgID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}
