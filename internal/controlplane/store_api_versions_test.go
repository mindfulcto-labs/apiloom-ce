package controlplane

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// mockAPIVersionStore implements APIVersionStore for handler tests.
type mockAPIVersionStore struct {
	listFn        func(ctx context.Context, routeID uuid.UUID) ([]domain.APIVersion, error)
	createFn      func(ctx context.Context, version *domain.APIVersion) error
	getFn         func(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error)
	setCanaryFn   func(ctx context.Context, canary *domain.CanaryConfig) error
	getCanaryFn   func(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error)
	clearCanaryFn func(ctx context.Context, routeID uuid.UUID) error
}

func (m *mockAPIVersionStore) ListAPIVersions(ctx context.Context, routeID uuid.UUID) ([]domain.APIVersion, error) {
	if m.listFn != nil {
		return m.listFn(ctx, routeID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockAPIVersionStore) CreateAPIVersion(ctx context.Context, version *domain.APIVersion) error {
	if m.createFn != nil {
		return m.createFn(ctx, version)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockAPIVersionStore) GetAPIVersion(ctx context.Context, versionID uuid.UUID) (*domain.APIVersion, error) {
	if m.getFn != nil {
		return m.getFn(ctx, versionID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockAPIVersionStore) SetCanary(ctx context.Context, canary *domain.CanaryConfig) error {
	if m.setCanaryFn != nil {
		return m.setCanaryFn(ctx, canary)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockAPIVersionStore) GetCanary(ctx context.Context, routeID uuid.UUID) (*domain.CanaryConfig, error) {
	if m.getCanaryFn != nil {
		return m.getCanaryFn(ctx, routeID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockAPIVersionStore) ClearCanary(ctx context.Context, routeID uuid.UUID) error {
	if m.clearCanaryFn != nil {
		return m.clearCanaryFn(ctx, routeID)
	}
	return fmt.Errorf("not implemented")
}

func TestAPIVersionStore_Interface(t *testing.T) {
	t.Parallel()

	// Verify nopAPIVersionStore satisfies the interface.
	var _ APIVersionStore = &nopAPIVersionStore{}
	var _ APIVersionStore = &mockAPIVersionStore{}
}

func TestAPIVersionStore_NopListVersions(t *testing.T) {
	t.Parallel()
	store := &nopAPIVersionStore{}
	versions, err := store.ListAPIVersions(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestAPIVersionStore_NopGetCanary(t *testing.T) {
	t.Parallel()
	store := &nopAPIVersionStore{}
	canary, err := store.GetCanary(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, canary)
}

func TestAPIVersionStore_NopGetVersion(t *testing.T) {
	t.Parallel()
	store := &nopAPIVersionStore{}
	v, err := store.GetAPIVersion(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, v)
}
