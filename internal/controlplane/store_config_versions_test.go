package controlplane

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/config"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// mockConfigVersionStore implements ConfigVersionStore for unit tests.
type mockConfigVersionStore struct {
	versions []ConfigVersion
	nextID   int
}

func newMockConfigVersionStore() *mockConfigVersionStore {
	return &mockConfigVersionStore{}
}

func (m *mockConfigVersionStore) CreateVersion(ctx context.Context, snapshot json.RawMessage, author, description string) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Find max version for this org.
	maxVer := 0
	for _, v := range m.versions {
		if v.OrganizationID == orgID && v.Version > maxVer {
			maxVer = v.Version
		}
	}

	m.nextID++
	cv := ConfigVersion{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Version:        maxVer + 1,
		Snapshot:       snapshot,
		Author:         author,
		Description:    description,
		CreatedAt:      time.Now().UTC(),
	}
	m.versions = append(m.versions, cv)
	return &cv, nil
}

func (m *mockConfigVersionStore) ListVersions(ctx context.Context, params ListParams) (*PagedResponse[ConfigVersion], error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []ConfigVersion
	for i := len(m.versions) - 1; i >= 0; i-- {
		if m.versions[i].OrganizationID == orgID {
			filtered = append(filtered, m.versions[i])
		}
	}

	limit := params.EffectiveLimit()
	return buildPagedResponse(filtered, limit, func(v ConfigVersion) (time.Time, uuid.UUID) {
		return v.CreatedAt, v.ID
	}), nil
}

func (m *mockConfigVersionStore) GetVersion(ctx context.Context, version int) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, err
	}

	for _, v := range m.versions {
		if v.OrganizationID == orgID && v.Version == version {
			return &v, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockConfigVersionStore) GetLatestVersion(ctx context.Context) (*ConfigVersion, error) {
	orgID, err := config.OrgFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var latest *ConfigVersion
	for i := range m.versions {
		v := &m.versions[i]
		if v.OrganizationID == orgID {
			if latest == nil || v.Version > latest.Version {
				latest = v
			}
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	return latest, nil
}

func (m *mockConfigVersionStore) RollbackToVersion(ctx context.Context, targetVersion int) (*ConfigVersion, error) {
	target, err := m.GetVersion(ctx, targetVersion)
	if err != nil {
		return nil, err
	}
	author := "rollback:v" + string(rune('0'+targetVersion))
	return m.CreateVersion(ctx, target.Snapshot, author, "rollback to version "+string(rune('0'+targetVersion)))
}

func TestConfigVersion_CreateFirst(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot := json.RawMessage(`{"routes": []}`)
	cv, err := store.CreateVersion(ctx, snapshot, "admin", "initial config")
	require.NoError(t, err)

	assert.Equal(t, 1, cv.Version)
	assert.Equal(t, orgID, cv.OrganizationID)
	assert.Equal(t, "admin", cv.Author)
	assert.Equal(t, "initial config", cv.Description)
	assert.JSONEq(t, `{"routes": []}`, string(cv.Snapshot))
}

func TestConfigVersion_CreateSecond(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot1 := json.RawMessage(`{"routes": []}`)
	_, err := store.CreateVersion(ctx, snapshot1, "admin", "v1")
	require.NoError(t, err)

	snapshot2 := json.RawMessage(`{"routes": [{"name": "foo"}]}`)
	cv2, err := store.CreateVersion(ctx, snapshot2, "admin", "v2")
	require.NoError(t, err)

	assert.Equal(t, 2, cv2.Version)
}

func TestConfigVersion_PerOrgVersionNumbers(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgA := uuid.New()
	orgB := uuid.New()
	ctxA := config.ContextWithOrg(context.Background(), orgA)
	ctxB := config.ContextWithOrg(context.Background(), orgB)

	snapshot := json.RawMessage(`{}`)

	cvA1, err := store.CreateVersion(ctxA, snapshot, "admin", "orgA v1")
	require.NoError(t, err)
	assert.Equal(t, 1, cvA1.Version)

	cvB1, err := store.CreateVersion(ctxB, snapshot, "admin", "orgB v1")
	require.NoError(t, err)
	assert.Equal(t, 1, cvB1.Version)

	cvA2, err := store.CreateVersion(ctxA, snapshot, "admin", "orgA v2")
	require.NoError(t, err)
	assert.Equal(t, 2, cvA2.Version)
}

func TestConfigVersion_ListVersions(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot := json.RawMessage(`{}`)
	for i := 0; i < 3; i++ {
		_, err := store.CreateVersion(ctx, snapshot, "admin", "")
		require.NoError(t, err)
	}

	result, err := store.ListVersions(ctx, ListParams{Limit: 10})
	require.NoError(t, err)

	assert.Len(t, result.Data, 3)
	// Newest first.
	assert.Equal(t, 3, result.Data[0].Version)
	assert.Equal(t, 2, result.Data[1].Version)
	assert.Equal(t, 1, result.Data[2].Version)
	assert.False(t, result.HasMore)
}

func TestConfigVersion_GetVersion(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot := json.RawMessage(`{"key": "value"}`)
	_, err := store.CreateVersion(ctx, snapshot, "admin", "test")
	require.NoError(t, err)

	cv, err := store.GetVersion(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, cv.Version)
	assert.JSONEq(t, `{"key": "value"}`, string(cv.Snapshot))
}

func TestConfigVersion_GetVersion_NotFound(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	_, err := store.GetVersion(ctx, 999)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConfigVersion_GetLatestVersion(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot := json.RawMessage(`{}`)
	_, err := store.CreateVersion(ctx, snapshot, "admin", "first")
	require.NoError(t, err)
	_, err = store.CreateVersion(ctx, snapshot, "admin", "second")
	require.NoError(t, err)

	latest, err := store.GetLatestVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, latest.Version)
	assert.Equal(t, "second", latest.Description)
}

func TestConfigVersion_GetLatestVersion_Empty(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	_, err := store.GetLatestVersion(ctx)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConfigVersion_RollbackCreatesNewVersion(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	snapshot1 := json.RawMessage(`{"routes": ["v1"]}`)
	_, err := store.CreateVersion(ctx, snapshot1, "admin", "v1")
	require.NoError(t, err)

	snapshot2 := json.RawMessage(`{"routes": ["v2"]}`)
	_, err = store.CreateVersion(ctx, snapshot2, "admin", "v2")
	require.NoError(t, err)

	// Rollback to version 1.
	rolled, err := store.RollbackToVersion(ctx, 1)
	require.NoError(t, err)

	// Rollback creates version 3 (new), not deletes version 2.
	assert.Equal(t, 3, rolled.Version)
	assert.JSONEq(t, `{"routes": ["v1"]}`, string(rolled.Snapshot))

	// Original versions still exist.
	v1, err := store.GetVersion(ctx, 1)
	require.NoError(t, err)
	assert.JSONEq(t, `{"routes": ["v1"]}`, string(v1.Snapshot))

	v2, err := store.GetVersion(ctx, 2)
	require.NoError(t, err)
	assert.JSONEq(t, `{"routes": ["v2"]}`, string(v2.Snapshot))
}

func TestConfigVersion_RollbackToNonExistent(t *testing.T) {
	t.Parallel()

	store := newMockConfigVersionStore()
	orgID := uuid.New()
	ctx := config.ContextWithOrg(context.Background(), orgID)

	_, err := store.RollbackToVersion(ctx, 999)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
