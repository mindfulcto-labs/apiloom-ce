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

	"github.com/cto-venkat/apiloom-ce/internal/config"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

func newTestEnvHandler(store EnvironmentStore) *EnvironmentHandler {
	return &EnvironmentHandler{store: store}
}

func envRequestWithOrg(method, path string, body []byte, orgID uuid.UUID) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := config.ContextWithOrg(r.Context(), orgID)
	return r.WithContext(ctx)
}

func TestEnvironmentHandler_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	now := time.Now().UTC()

	tests := []struct {
		name       string
		store      *mockEnvironmentStore
		wantStatus int
		wantCount  int
	}{
		{
			name: "returns environments",
			store: &mockEnvironmentStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.Environment, error) {
					return []domain.Environment{
						{ID: uuid.New(), OrganizationID: orgID, Name: "dev", Slug: "dev", CreatedAt: now, UpdatedAt: now},
						{ID: uuid.New(), OrganizationID: orgID, Name: "prod", Slug: "prod", CreatedAt: now, UpdatedAt: now},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "empty list",
			store: &mockEnvironmentStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.Environment, error) {
					return []domain.Environment{}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "store error",
			store: &mockEnvironmentStore{
				listFn: func(_ context.Context, _ uuid.UUID) ([]domain.Environment, error) {
					return nil, fmt.Errorf("db down")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestEnvHandler(tt.store)
			w := httptest.NewRecorder()
			r := envRequestWithOrg(http.MethodGet, "/api/v1/environments", nil, orgID)
			handler.HandleListEnvironments(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				var envs []domain.Environment
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envs))
				assert.Len(t, envs, tt.wantCount)
			}
		})
	}
}

func TestEnvironmentHandler_Create(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()

	tests := []struct {
		name       string
		body       string
		store      *mockEnvironmentStore
		wantStatus int
	}{
		{
			name: "valid create returns 201",
			body: `{"name":"Test Env","description":"A test environment"}`,
			store: &mockEnvironmentStore{
				createFn: func(_ context.Context, _ *domain.Environment) error {
					return nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid name returns 400",
			body:       `{"name":"x"}`,
			store:      &mockEnvironmentStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "store error returns 500",
			body: `{"name":"Good Name","description":"desc"}`,
			store: &mockEnvironmentStore{
				createFn: func(_ context.Context, _ *domain.Environment) error {
					return fmt.Errorf("db error")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "empty body returns 400",
			body:       `{}`,
			store:      &mockEnvironmentStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestEnvHandler(tt.store)
			w := httptest.NewRecorder()
			r := envRequestWithOrg(http.MethodPost, "/api/v1/environments", []byte(tt.body), orgID)
			handler.HandleCreateEnvironment(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestEnvironmentHandler_Delete(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	envID := uuid.New()
	prodEnvID := uuid.New()

	tests := []struct {
		name       string
		envID      string
		store      *mockEnvironmentStore
		wantStatus int
	}{
		{
			name:  "successful delete returns 204",
			envID: envID.String(),
			store: &mockEnvironmentStore{
				deleteFn: func(_ context.Context, _, _ uuid.UUID) error {
					return nil
				},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:  "production env returns 403",
			envID: prodEnvID.String(),
			store: &mockEnvironmentStore{
				deleteFn: func(_ context.Context, _, _ uuid.UUID) error {
					return fmt.Errorf("cannot delete production: %w", domain.ErrForbidden)
				},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "invalid UUID returns 400",
			envID:      "not-a-uuid",
			store:      &mockEnvironmentStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestEnvHandler(tt.store)

			router := chi.NewRouter()
			router.Delete("/environments/{id}", handler.handleDelete)

			w := httptest.NewRecorder()
			r := envRequestWithOrg(http.MethodDelete, "/environments/"+tt.envID, nil, orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestEnvironmentHandler_Promote(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()

	tests := []struct {
		name       string
		sourceID   string
		body       string
		store      *mockEnvironmentStore
		wantStatus int
	}{
		{
			name:     "successful promotion returns 200",
			sourceID: sourceID.String(),
			body:     fmt.Sprintf(`{"target_environment_id":"%s"}`, targetID),
			store: &mockEnvironmentStore{
				promoteFn: func(_ context.Context, _, _, _ uuid.UUID) (*domain.PromotionResult, error) {
					return &domain.PromotionResult{
						RoutesCreated:   3,
						BackendsCreated: 1,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "same source and target",
			sourceID: sourceID.String(),
			body:     fmt.Sprintf(`{"target_environment_id":"%s"}`, sourceID),
			store: &mockEnvironmentStore{
				promoteFn: func(_ context.Context, _, _, _ uuid.UUID) (*domain.PromotionResult, error) {
					return nil, fmt.Errorf("same env: %w", domain.ErrInvalidConfig)
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid target UUID returns 400",
			sourceID:   sourceID.String(),
			body:       `{"target_environment_id":"not-a-uuid"}`,
			store:      &mockEnvironmentStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := newTestEnvHandler(tt.store)

			router := chi.NewRouter()
			router.Post("/environments/{id}/promote", handler.HandlePromoteEnvironment)

			w := httptest.NewRecorder()
			r := envRequestWithOrg(http.MethodPost, "/environments/"+tt.sourceID+"/promote", []byte(tt.body), orgID)
			router.ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var result domain.PromotionResult
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				assert.Greater(t, result.RoutesCreated+result.RoutesUpdated+result.BackendsCreated+result.BackendsUpdated, 0)
			}
		})
	}
}
