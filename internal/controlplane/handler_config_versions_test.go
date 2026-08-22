package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// mockCVStore implements ConfigVersionStore for handler tests.
type mockCVStore struct {
	listVersionsFunc    func(ctx context.Context, params ListParams) (*PagedResponse[ConfigVersion], error)
	getVersionFunc      func(ctx context.Context, version int) (*ConfigVersion, error)
	rollbackToVersionFn func(ctx context.Context, targetVersion int) (*ConfigVersion, error)
}

func (m *mockCVStore) CreateVersion(ctx context.Context, snapshot json.RawMessage, author, description string) (*ConfigVersion, error) {
	return nil, nil
}

func (m *mockCVStore) ListVersions(ctx context.Context, params ListParams) (*PagedResponse[ConfigVersion], error) {
	if m.listVersionsFunc != nil {
		return m.listVersionsFunc(ctx, params)
	}
	return &PagedResponse[ConfigVersion]{Data: []ConfigVersion{}}, nil
}

func (m *mockCVStore) GetVersion(ctx context.Context, version int) (*ConfigVersion, error) {
	if m.getVersionFunc != nil {
		return m.getVersionFunc(ctx, version)
	}
	return nil, domain.ErrNotFound
}

func (m *mockCVStore) GetLatestVersion(ctx context.Context) (*ConfigVersion, error) {
	return nil, domain.ErrNotFound
}

func (m *mockCVStore) RollbackToVersion(ctx context.Context, targetVersion int) (*ConfigVersion, error) {
	if m.rollbackToVersionFn != nil {
		return m.rollbackToVersionFn(ctx, targetVersion)
	}
	return nil, domain.ErrNotFound
}

func newTestConfigVersionRouter(store ConfigVersionStore) http.Handler {
	h := &ConfigVersionHandler{store: store}
	r := chi.NewRouter()
	r.Route("/config", h.Routes)
	return r
}

func orgCtx(next http.Handler, orgID uuid.UUID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := config.ContextWithOrg(r.Context(), orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestConfigVersionHandler_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	orgID := uuid.New()

	store := &mockCVStore{
		listVersionsFunc: func(_ context.Context, _ ListParams) (*PagedResponse[ConfigVersion], error) {
			return &PagedResponse[ConfigVersion]{
				Data: []ConfigVersion{
					{ID: uuid.New(), OrganizationID: orgID, Version: 2, Author: "admin", Description: "v2", CreatedAt: now},
					{ID: uuid.New(), OrganizationID: orgID, Version: 1, Author: "admin", Description: "v1", CreatedAt: now.Add(-time.Hour)},
				},
			}, nil
		},
	}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	req := httptest.NewRequest(http.MethodGet, "/config/versions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp PagedResponse[ConfigVersionListItem]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, 2, resp.Data[0].Version)
	assert.Equal(t, "admin", resp.Data[0].Author)
}

func TestConfigVersionHandler_Get(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	orgID := uuid.New()
	snapshot := json.RawMessage(`{"routes": []}`)

	store := &mockCVStore{
		getVersionFunc: func(_ context.Context, version int) (*ConfigVersion, error) {
			if version == 1 {
				return &ConfigVersion{
					ID: uuid.New(), OrganizationID: orgID, Version: 1,
					Snapshot: snapshot, Author: "admin", Description: "initial",
					CreatedAt: now,
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	req := httptest.NewRequest(http.MethodGet, "/config/versions/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var cv ConfigVersion
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cv))
	assert.Equal(t, 1, cv.Version)
	assert.JSONEq(t, `{"routes": []}`, string(cv.Snapshot))
}

func TestConfigVersionHandler_Get_NotFound(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := &mockCVStore{}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	req := httptest.NewRequest(http.MethodGet, "/config/versions/999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestConfigVersionHandler_Rollback(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	orgID := uuid.New()
	snapshot := json.RawMessage(`{"routes": ["v1"]}`)

	store := &mockCVStore{
		rollbackToVersionFn: func(_ context.Context, targetVersion int) (*ConfigVersion, error) {
			if targetVersion == 1 {
				return &ConfigVersion{
					ID: uuid.New(), OrganizationID: orgID, Version: 3,
					Snapshot: snapshot, Author: "rollback:v1", Description: "rollback to version 1",
					CreatedAt: now,
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	body := bytes.NewBufferString(`{"to_version": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/config/rollback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var cv ConfigVersion
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cv))
	assert.Equal(t, 3, cv.Version)
	assert.Equal(t, "rollback:v1", cv.Author)
}

func TestConfigVersionHandler_Rollback_InvalidVersion(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := &mockCVStore{
		rollbackToVersionFn: func(_ context.Context, _ int) (*ConfigVersion, error) {
			return nil, domain.ErrNotFound
		},
	}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	body := bytes.NewBufferString(`{"to_version": 999}`)
	req := httptest.NewRequest(http.MethodPost, "/config/rollback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestConfigVersionHandler_Rollback_BadRequest(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := &mockCVStore{}

	router := orgCtx(newTestConfigVersionRouter(store), orgID)
	body := bytes.NewBufferString(`{"invalid": true}`)
	req := httptest.NewRequest(http.MethodPost, "/config/rollback", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
