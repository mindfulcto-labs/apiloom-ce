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

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

func newTestAPIVersionHandler(store APIVersionStore) *APIVersionHandler {
	return NewAPIVersionHandler(store)
}

func apiVersionRouter(handler *APIVersionHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/routes/{id}/versions", handler.Routes)
	r.Route("/routes/{id}/canary", handler.CanaryRoutes)
	return r
}

func apiVersionRequest(method, path string, body []byte, orgID uuid.UUID) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := config.ContextWithOrg(r.Context(), orgID)
	return r.WithContext(ctx)
}

func TestAPIVersionHandler_ListVersions(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()
	now := time.Now().UTC()

	tests := []struct {
		name       string
		store      *mockAPIVersionStore
		wantStatus int
		wantCount  int
	}{
		{
			name: "returns versions",
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return []domain.APIVersion{
						{ID: uuid.New(), RouteID: routeID, VersionTag: "v1.0.0", Config: json.RawMessage(`{}`), CreatedAt: now},
						{ID: uuid.New(), RouteID: routeID, VersionTag: "v1.1.0", Config: json.RawMessage(`{}`), CreatedAt: now},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "empty list",
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return []domain.APIVersion{}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "store error",
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return nil, fmt.Errorf("db down")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestAPIVersionHandler(tt.store)
			router := apiVersionRouter(handler)
			w := httptest.NewRecorder()
			r := apiVersionRequest(http.MethodGet, "/routes/"+routeID.String()+"/versions", nil, orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				var versions []domain.APIVersion
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &versions))
				assert.Len(t, versions, tt.wantCount)
			}
		})
	}
}

func TestAPIVersionHandler_CreateVersion(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()

	tests := []struct {
		name       string
		body       string
		store      *mockAPIVersionStore
		wantStatus int
	}{
		{
			name: "valid create returns 201",
			body: fmt.Sprintf(`{"version_tag":"v1.0.0","config":{"timeout":30},"changelog":"initial","created_by":"user1"}`),
			store: &mockAPIVersionStore{
				createFn: func(_ context.Context, _ *domain.APIVersion) error {
					return nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid semver returns 400",
			body:       `{"version_tag":"bad","config":{}}`,
			store:      &mockAPIVersionStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "store error returns 500",
			body: `{"version_tag":"v1.0.0","config":{}}`,
			store: &mockAPIVersionStore{
				createFn: func(_ context.Context, _ *domain.APIVersion) error {
					return fmt.Errorf("db error")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestAPIVersionHandler(tt.store)
			router := apiVersionRouter(handler)
			w := httptest.NewRecorder()
			r := apiVersionRequest(http.MethodPost, "/routes/"+routeID.String()+"/versions", []byte(tt.body), orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAPIVersionHandler_DiffVersions(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()
	v1ID := uuid.New()
	v2ID := uuid.New()

	tests := []struct {
		name       string
		vid        string
		compareTo  string
		store      *mockAPIVersionStore
		wantStatus int
	}{
		{
			name:      "valid diff returns 200",
			vid:       v1ID.String(),
			compareTo: v2ID.String(),
			store: &mockAPIVersionStore{
				getFn: func(_ context.Context, id uuid.UUID) (*domain.APIVersion, error) {
					if id == v1ID {
						return &domain.APIVersion{ID: v1ID, Config: json.RawMessage(`{"timeout":30}`)}, nil
					}
					return &domain.APIVersion{ID: v2ID, Config: json.RawMessage(`{"timeout":60,"retry":3}`)}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "missing compare_to returns 400",
			vid:       v1ID.String(),
			compareTo: "",
			store:     &mockAPIVersionStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "invalid compare_to returns 400",
			vid:       v1ID.String(),
			compareTo: "not-uuid",
			store:     &mockAPIVersionStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "version not found returns 404",
			vid:       v1ID.String(),
			compareTo: v2ID.String(),
			store: &mockAPIVersionStore{
				getFn: func(_ context.Context, _ uuid.UUID) (*domain.APIVersion, error) {
					return nil, fmt.Errorf("not found: %w", domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestAPIVersionHandler(tt.store)
			router := apiVersionRouter(handler)
			w := httptest.NewRecorder()
			path := "/routes/" + routeID.String() + "/versions/" + tt.vid + "/diff"
			if tt.compareTo != "" {
				path += "?compare_to=" + tt.compareTo
			}
			r := apiVersionRequest(http.MethodGet, path, nil, orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var resp diffResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.NotNil(t, resp.Diff)
				assert.NotNil(t, resp.MigrationNotes)
			}
		})
	}
}

func TestAPIVersionHandler_SetCanary(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()
	v1ID := uuid.New()
	v2ID := uuid.New()

	tests := []struct {
		name       string
		body       string
		store      *mockAPIVersionStore
		wantStatus int
	}{
		{
			name: "valid set canary returns 200",
			body: fmt.Sprintf(`{"canary_version_id":"%s","weight":25}`, v2ID),
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return []domain.APIVersion{
						{ID: v1ID, VersionTag: "v1.0.0"},
						{ID: v2ID, VersionTag: "v2.0.0"},
					}, nil
				},
				setCanaryFn: func(_ context.Context, _ *domain.CanaryConfig) error {
					return nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "weight over 100 returns 400",
			body: fmt.Sprintf(`{"canary_version_id":"%s","weight":150}`, v2ID),
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return []domain.APIVersion{
						{ID: v1ID, VersionTag: "v1.0.0"},
						{ID: v2ID, VersionTag: "v2.0.0"},
					}, nil
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "no versions returns 400",
			body: fmt.Sprintf(`{"canary_version_id":"%s","weight":25}`, v2ID),
			store: &mockAPIVersionStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.APIVersion, error) {
					return []domain.APIVersion{}, nil
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestAPIVersionHandler(tt.store)
			router := apiVersionRouter(handler)
			w := httptest.NewRecorder()
			r := apiVersionRequest(http.MethodPut, "/routes/"+routeID.String()+"/canary", []byte(tt.body), orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAPIVersionHandler_ClearCanary(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()

	store := &mockAPIVersionStore{
		clearCanaryFn: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}

	handler := newTestAPIVersionHandler(store)
	router := apiVersionRouter(handler)
	w := httptest.NewRecorder()
	r := apiVersionRequest(http.MethodDelete, "/routes/"+routeID.String()+"/canary", nil, orgID)
	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAPIVersionHandler_GetCanary(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	routeID := uuid.New()

	tests := []struct {
		name       string
		store      *mockAPIVersionStore
		wantStatus int
	}{
		{
			name: "active canary returns 200",
			store: &mockAPIVersionStore{
				getCanaryFn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
					return &domain.CanaryConfig{
						RouteID:      routeID,
						CanaryWeight: 25,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "no canary returns 200 with null",
			store: &mockAPIVersionStore{
				getCanaryFn: func(_ context.Context, _ uuid.UUID) (*domain.CanaryConfig, error) {
					return nil, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestAPIVersionHandler(tt.store)
			router := apiVersionRouter(handler)
			w := httptest.NewRecorder()
			r := apiVersionRequest(http.MethodGet, "/routes/"+routeID.String()+"/canary", nil, orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
