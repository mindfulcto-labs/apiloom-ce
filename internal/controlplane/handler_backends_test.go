package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// --- Mock stores for backend handler tests ---

type mockBackendConfigStore struct {
	saveBackendFunc   func(ctx context.Context, bc *domain.BackendConfig) (*domain.BackendConfig, error)
	loadBackendFunc   func(ctx context.Context, id uuid.UUID) (*domain.BackendConfig, error)
	deleteBackendFunc func(ctx context.Context, id uuid.UUID) error
}

func (m *mockBackendConfigStore) SaveBackend(ctx context.Context, bc *domain.BackendConfig) (*domain.BackendConfig, error) {
	if m.saveBackendFunc != nil {
		return m.saveBackendFunc(ctx, bc)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBackendConfigStore) LoadBackend(ctx context.Context, id uuid.UUID) (*domain.BackendConfig, error) {
	if m.loadBackendFunc != nil {
		return m.loadBackendFunc(ctx, id)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBackendConfigStore) DeleteBackend(ctx context.Context, id uuid.UUID) error {
	if m.deleteBackendFunc != nil {
		return m.deleteBackendFunc(ctx, id)
	}
	return fmt.Errorf("not implemented")
}

type mockBackendStore struct {
	updateBackendFunc    func(ctx context.Context, id uuid.UUID, req UpdateBackendRequest) (*domain.BackendConfig, error)
	listBackendsPagedFn  func(ctx context.Context, params ListParams) (*PagedResponse[domain.BackendConfig], error)
}

func (m *mockBackendStore) UpdateBackend(ctx context.Context, id uuid.UUID, req UpdateBackendRequest) (*domain.BackendConfig, error) {
	if m.updateBackendFunc != nil {
		return m.updateBackendFunc(ctx, id, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBackendStore) ListBackendsPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.BackendConfig], error) {
	if m.listBackendsPagedFn != nil {
		return m.listBackendsPagedFn(ctx, params)
	}
	return nil, fmt.Errorf("not implemented")
}

func newTestBackendHandler(cfgStore BackendConfigStore, store BackendStore) *BackendHandler {
	return &BackendHandler{
		configStore: cfgStore,
		store:       store,
	}
}

func TestBackendHandler_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	backendID := uuid.New()

	tests := []struct {
		name       string
		body       string
		cfgStore   *mockBackendConfigStore
		wantStatus int
	}{
		{
			name: "valid backend returns 201",
			body: `{"name":"test-backend","config":{"url":"http://localhost"}}`,
			cfgStore: &mockBackendConfigStore{
				saveBackendFunc: func(_ context.Context, bc *domain.BackendConfig) (*domain.BackendConfig, error) {
					return &domain.BackendConfig{
						ID: backendID, Name: bc.Name,
						Config: bc.Config, Metadata: bc.Metadata,
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name returns 400",
			body:       `{"name":"ab","config":{"url":"http://localhost"}}`,
			cfgStore:   &mockBackendConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing config returns 400",
			body:       `{"name":"valid-backend"}`,
			cfgStore:   &mockBackendConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate returns 409",
			body: `{"name":"dup-backend","config":{"url":"http://localhost"}}`,
			cfgStore: &mockBackendConfigStore{
				saveBackendFunc: func(_ context.Context, _ *domain.BackendConfig) (*domain.BackendConfig, error) {
					return nil, fmt.Errorf("backend name %q: %w", "dup-backend", domain.ErrAlreadyExists)
				},
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestBackendHandler(tt.cfgStore, &mockBackendStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				var resp BackendResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, backendID, resp.ID)
			}
		})
	}
}

func TestBackendHandler_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	backendID := uuid.New()

	tests := []struct {
		name       string
		id         string
		cfgStore   *mockBackendConfigStore
		wantStatus int
	}{
		{
			name: "existing returns 200",
			id:   backendID.String(),
			cfgStore: &mockBackendConfigStore{
				loadBackendFunc: func(_ context.Context, id uuid.UUID) (*domain.BackendConfig, error) {
					return &domain.BackendConfig{
						ID: id, Name: "test",
						Config: json.RawMessage(`{}`),
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "bad UUID returns 400",
			id:         "not-uuid",
			cfgStore:   &mockBackendConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			cfgStore: &mockBackendConfigStore{
				loadBackendFunc: func(_ context.Context, id uuid.UUID) (*domain.BackendConfig, error) {
					return nil, fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestBackendHandler(tt.cfgStore, &mockBackendStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodGet, "/"+tt.id, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBackendHandler_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	store := &mockBackendStore{
		listBackendsPagedFn: func(_ context.Context, _ ListParams) (*PagedResponse[domain.BackendConfig], error) {
			return &PagedResponse[domain.BackendConfig]{
				Data: []domain.BackendConfig{
					{ID: uuid.New(), Name: "b1", Config: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}

	h := newTestBackendHandler(&mockBackendConfigStore{}, store)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp PagedResponse[BackendResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
}

func TestBackendHandler_Update(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	backendID := uuid.New()

	tests := []struct {
		name       string
		id         string
		body       string
		store      *mockBackendStore
		wantStatus int
	}{
		{
			name: "valid update returns 200",
			id:   backendID.String(),
			body: `{"name":"updated-backend","config":{"url":"http://new"}}`,
			store: &mockBackendStore{
				updateBackendFunc: func(_ context.Context, id uuid.UUID, req UpdateBackendRequest) (*domain.BackendConfig, error) {
					return &domain.BackendConfig{
						ID: id, Name: req.Name,
						Config: req.Config,
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "bad UUID returns 400",
			id:         "bad",
			body:       `{"name":"updated","config":{}}`,
			store:      &mockBackendStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			body: `{"name":"updated-backend","config":{"url":"http://new"}}`,
			store: &mockBackendStore{
				updateBackendFunc: func(_ context.Context, id uuid.UUID, _ UpdateBackendRequest) (*domain.BackendConfig, error) {
					return nil, fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestBackendHandler(&mockBackendConfigStore{}, tt.store)
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodPut, "/"+tt.id, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBackendHandler_Delete(t *testing.T) {
	t.Parallel()

	backendID := uuid.New()

	tests := []struct {
		name       string
		id         string
		cfgStore   *mockBackendConfigStore
		wantStatus int
	}{
		{
			name: "existing returns 204",
			id:   backendID.String(),
			cfgStore: &mockBackendConfigStore{
				deleteBackendFunc: func(_ context.Context, _ uuid.UUID) error {
					return nil
				},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "bad UUID returns 400",
			id:         "bad",
			cfgStore:   &mockBackendConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			cfgStore: &mockBackendConfigStore{
				deleteBackendFunc: func(_ context.Context, id uuid.UUID) error {
					return fmt.Errorf("backend %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestBackendHandler(tt.cfgStore, &mockBackendStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodDelete, "/"+tt.id, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
