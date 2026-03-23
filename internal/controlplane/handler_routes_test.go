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

// --- Mock stores for handler tests ---

type mockRouteConfigStore struct {
	saveRouteFunc   func(ctx context.Context, rc *domain.RouteConfig) (*domain.RouteConfig, error)
	loadRouteFunc   func(ctx context.Context, id uuid.UUID) (*domain.RouteConfig, error)
	deleteRouteFunc func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRouteConfigStore) SaveRoute(ctx context.Context, rc *domain.RouteConfig) (*domain.RouteConfig, error) {
	if m.saveRouteFunc != nil {
		return m.saveRouteFunc(ctx, rc)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRouteConfigStore) LoadRoute(ctx context.Context, id uuid.UUID) (*domain.RouteConfig, error) {
	if m.loadRouteFunc != nil {
		return m.loadRouteFunc(ctx, id)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRouteConfigStore) DeleteRoute(ctx context.Context, id uuid.UUID) error {
	if m.deleteRouteFunc != nil {
		return m.deleteRouteFunc(ctx, id)
	}
	return fmt.Errorf("not implemented")
}

type mockRouteStore struct {
	updateRouteFunc    func(ctx context.Context, id uuid.UUID, req UpdateRouteRequest) (*domain.RouteConfig, error)
	listRoutesPagedFn  func(ctx context.Context, params ListParams) (*PagedResponse[domain.RouteConfig], error)
}

func (m *mockRouteStore) UpdateRoute(ctx context.Context, id uuid.UUID, req UpdateRouteRequest) (*domain.RouteConfig, error) {
	if m.updateRouteFunc != nil {
		return m.updateRouteFunc(ctx, id, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRouteStore) ListRoutesPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.RouteConfig], error) {
	if m.listRoutesPagedFn != nil {
		return m.listRoutesPagedFn(ctx, params)
	}
	return nil, fmt.Errorf("not implemented")
}

func newTestRouteHandler(cfgStore RouteConfigStore, store RouteStore) *RouteHandler {
	return &RouteHandler{
		configStore: cfgStore,
		store:       store,
	}
}

func TestRouteHandler_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	routeID := uuid.New()

	tests := []struct {
		name       string
		body       string
		cfgStore   *mockRouteConfigStore
		wantStatus int
		wantType   string // for error responses, check type field
	}{
		{
			name: "valid route returns 201",
			body: `{"name":"test-route","config":{"path":"/api"}}`,
			cfgStore: &mockRouteConfigStore{
				saveRouteFunc: func(_ context.Context, rc *domain.RouteConfig) (*domain.RouteConfig, error) {
					return &domain.RouteConfig{
						ID: routeID, Name: rc.Name, Version: 1,
						Config: rc.Config, Metadata: rc.Metadata,
						Checksum: "abc123", IsActive: true,
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name returns 400",
			body:       `{"name":"ab","config":{"path":"/api"}}`,
			cfgStore:   &mockRouteConfigStore{},
			wantStatus: http.StatusBadRequest,
			wantType:   "urn:flexgateway:error:validation",
		},
		{
			name:       "missing config returns 400",
			body:       `{"name":"valid-name"}`,
			cfgStore:   &mockRouteConfigStore{},
			wantStatus: http.StatusBadRequest,
			wantType:   "urn:flexgateway:error:validation",
		},
		{
			name:       "invalid JSON body returns 400",
			body:       `{broken`,
			cfgStore:   &mockRouteConfigStore{},
			wantStatus: http.StatusBadRequest,
			wantType:   "urn:flexgateway:error:validation",
		},
		{
			name: "duplicate name returns 409",
			body: `{"name":"dup-route","config":{"path":"/api"}}`,
			cfgStore: &mockRouteConfigStore{
				saveRouteFunc: func(_ context.Context, _ *domain.RouteConfig) (*domain.RouteConfig, error) {
					return nil, fmt.Errorf("route name %q: %w", "dup-route", domain.ErrAlreadyExists)
				},
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestRouteHandler(tt.cfgStore, &mockRouteStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				var resp RouteResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, routeID, resp.ID)
			}
			if tt.wantType != "" {
				assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
				var prob domain.ProblemError
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prob))
				assert.Equal(t, tt.wantType, prob.Type)
			}
		})
	}
}

func TestRouteHandler_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	routeID := uuid.New()

	tests := []struct {
		name       string
		id         string
		cfgStore   *mockRouteConfigStore
		wantStatus int
	}{
		{
			name: "existing route returns 200",
			id:   routeID.String(),
			cfgStore: &mockRouteConfigStore{
				loadRouteFunc: func(_ context.Context, id uuid.UUID) (*domain.RouteConfig, error) {
					return &domain.RouteConfig{
						ID: id, Name: "test", Version: 1,
						Config: json.RawMessage(`{}`), IsActive: true,
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "bad UUID returns 400",
			id:         "not-a-uuid",
			cfgStore:   &mockRouteConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			cfgStore: &mockRouteConfigStore{
				loadRouteFunc: func(_ context.Context, id uuid.UUID) (*domain.RouteConfig, error) {
					return nil, fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestRouteHandler(tt.cfgStore, &mockRouteStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodGet, "/"+tt.id, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			}
			if tt.wantStatus == http.StatusNotFound {
				assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestRouteHandler_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	store := &mockRouteStore{
		listRoutesPagedFn: func(_ context.Context, params ListParams) (*PagedResponse[domain.RouteConfig], error) {
			return &PagedResponse[domain.RouteConfig]{
				Data: []domain.RouteConfig{
					{ID: uuid.New(), Name: "r1", Config: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}

	h := newTestRouteHandler(&mockRouteConfigStore{}, store)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp PagedResponse[RouteResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
	assert.False(t, resp.HasMore)
}

func TestRouteHandler_Update(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	routeID := uuid.New()

	tests := []struct {
		name       string
		id         string
		body       string
		store      *mockRouteStore
		wantStatus int
	}{
		{
			name: "valid update returns 200",
			id:   routeID.String(),
			body: `{"name":"updated-route","config":{"path":"/new"}}`,
			store: &mockRouteStore{
				updateRouteFunc: func(_ context.Context, id uuid.UUID, req UpdateRouteRequest) (*domain.RouteConfig, error) {
					return &domain.RouteConfig{
						ID: id, Name: req.Name, Version: 2,
						Config: req.Config, IsActive: true,
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
			store:      &mockRouteStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing name returns 400",
			id:         routeID.String(),
			body:       `{"name":"ab","config":{}}`,
			store:      &mockRouteStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			body: `{"name":"updated-route","config":{"path":"/new"}}`,
			store: &mockRouteStore{
				updateRouteFunc: func(_ context.Context, id uuid.UUID, _ UpdateRouteRequest) (*domain.RouteConfig, error) {
					return nil, fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestRouteHandler(&mockRouteConfigStore{}, tt.store)
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

func TestRouteHandler_Delete(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()

	tests := []struct {
		name       string
		id         string
		cfgStore   *mockRouteConfigStore
		wantStatus int
	}{
		{
			name: "existing route returns 204",
			id:   routeID.String(),
			cfgStore: &mockRouteConfigStore{
				deleteRouteFunc: func(_ context.Context, _ uuid.UUID) error {
					return nil
				},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "bad UUID returns 400",
			id:         "bad",
			cfgStore:   &mockRouteConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			cfgStore: &mockRouteConfigStore{
				deleteRouteFunc: func(_ context.Context, id uuid.UUID) error {
					return fmt.Errorf("route %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestRouteHandler(tt.cfgStore, &mockRouteStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodDelete, "/"+tt.id, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
