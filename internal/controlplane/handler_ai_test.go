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

// mockAIStore implements the store methods needed by AIHandler for unit testing.
type mockAIStore struct {
	createAIModelFn       func(ctx context.Context, req CreateAIModelRequest) (*AIModelRow, error)
	listAIModelsPagedFn   func(ctx context.Context, params ListParams) (*PagedResponse[AIModelRow], error)
	getAIModelFn          func(ctx context.Context, id uuid.UUID) (*AIModelRow, error)
	updateAIModelFn       func(ctx context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error)
	deleteAIModelFn       func(ctx context.Context, id uuid.UUID) error
	createTokenBudgetFn   func(ctx context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error)
	listTokenBudgetsPagedFn func(ctx context.Context, params ListParams) (*PagedResponse[TokenBudgetRow], error)
	getTokenBudgetFn      func(ctx context.Context, id uuid.UUID) (*TokenBudgetRow, error)
	updateTokenBudgetFn   func(ctx context.Context, id uuid.UUID, req UpdateTokenBudgetRequest) (*TokenBudgetRow, error)
	deleteTokenBudgetFn   func(ctx context.Context, id uuid.UUID) error
}

func (m *mockAIStore) CreateAIModel(ctx context.Context, req CreateAIModelRequest) (*AIModelRow, error) {
	return m.createAIModelFn(ctx, req)
}
func (m *mockAIStore) ListAIModelsPaged(ctx context.Context, params ListParams) (*PagedResponse[AIModelRow], error) {
	return m.listAIModelsPagedFn(ctx, params)
}
func (m *mockAIStore) GetAIModel(ctx context.Context, id uuid.UUID) (*AIModelRow, error) {
	return m.getAIModelFn(ctx, id)
}
func (m *mockAIStore) UpdateAIModel(ctx context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error) {
	return m.updateAIModelFn(ctx, id, req)
}
func (m *mockAIStore) DeleteAIModel(ctx context.Context, id uuid.UUID) error {
	return m.deleteAIModelFn(ctx, id)
}
func (m *mockAIStore) CreateTokenBudget(ctx context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error) {
	return m.createTokenBudgetFn(ctx, req)
}
func (m *mockAIStore) ListTokenBudgetsPaged(ctx context.Context, params ListParams) (*PagedResponse[TokenBudgetRow], error) {
	return m.listTokenBudgetsPagedFn(ctx, params)
}
func (m *mockAIStore) GetTokenBudget(ctx context.Context, id uuid.UUID) (*TokenBudgetRow, error) {
	return m.getTokenBudgetFn(ctx, id)
}
func (m *mockAIStore) UpdateTokenBudget(ctx context.Context, id uuid.UUID, req UpdateTokenBudgetRequest) (*TokenBudgetRow, error) {
	return m.updateTokenBudgetFn(ctx, id, req)
}
func (m *mockAIStore) DeleteTokenBudget(ctx context.Context, id uuid.UUID) error {
	return m.deleteTokenBudgetFn(ctx, id)
}

func newAITestRouter(store AIStoreInterface) *chi.Mux {
	h := NewAIHandler(store)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func TestAIModelCreate(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	modelID := uuid.New()

	tests := []struct {
		name       string
		body       any
		storeFn    func(ctx context.Context, req CreateAIModelRequest) (*AIModelRow, error)
		wantStatus int
	}{
		{
			name: "valid create returns 201",
			body: CreateAIModelRequest{Provider: "openai", ModelName: "gpt-4"},
			storeFn: func(_ context.Context, req CreateAIModelRequest) (*AIModelRow, error) {
				return &AIModelRow{
					ID: modelID, Provider: req.Provider, ModelName: req.ModelName,
					IsActive: true, Weight: 1, CreatedAt: now, UpdatedAt: now,
				}, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing provider returns 400",
			body:       CreateAIModelRequest{ModelName: "gpt-4"},
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing model_name returns 400",
			body:       CreateAIModelRequest{Provider: "openai"},
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate returns 409",
			body: CreateAIModelRequest{Provider: "openai", ModelName: "gpt-4"},
			storeFn: func(_ context.Context, _ CreateAIModelRequest) (*AIModelRow, error) {
				return nil, fmt.Errorf("ai model openai/gpt-4: %w", domain.ErrAlreadyExists)
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{
				createAIModelFn: tt.storeFn,
			}
			r := newAITestRouter(store)

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/ai/models", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				var got AIModelRow
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, modelID, got.ID)
			}
			if tt.wantStatus >= 400 {
				assert.Contains(t, w.Header().Get("Content-Type"), "application/problem+json")
			}
		})
	}
}

func TestAIModelList(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)

	store := &mockAIStore{
		listAIModelsPagedFn: func(_ context.Context, params ListParams) (*PagedResponse[AIModelRow], error) {
			return &PagedResponse[AIModelRow]{
				Data: []AIModelRow{
					{ID: uuid.New(), Provider: "openai", ModelName: "gpt-4", IsActive: true, CreatedAt: now, UpdatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}
	r := newAITestRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/ai/models?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got PagedResponse[AIModelRow]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Data, 1)
}

func TestAIModelGet(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	modelID := uuid.New()

	tests := []struct {
		name       string
		id         string
		storeFn    func(ctx context.Context, id uuid.UUID) (*AIModelRow, error)
		wantStatus int
	}{
		{
			name: "found returns 200",
			id:   modelID.String(),
			storeFn: func(_ context.Context, id uuid.UUID) (*AIModelRow, error) {
				return &AIModelRow{ID: id, Provider: "openai", ModelName: "gpt-4", IsActive: true, CreatedAt: now, UpdatedAt: now}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid UUID returns 400",
			id:         "not-a-uuid",
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			storeFn: func(_ context.Context, id uuid.UUID) (*AIModelRow, error) {
				return nil, fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{getAIModelFn: tt.storeFn}
			r := newAITestRouter(store)

			req := httptest.NewRequest(http.MethodGet, "/ai/models/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAIModelUpdate(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	modelID := uuid.New()

	tests := []struct {
		name       string
		id         string
		body       any
		storeFn    func(ctx context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error)
		wantStatus int
	}{
		{
			name: "valid update returns 200",
			id:   modelID.String(),
			body: UpdateAIModelRequest{Provider: "openai", ModelName: "gpt-4o"},
			storeFn: func(_ context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error) {
				return &AIModelRow{ID: id, Provider: req.Provider, ModelName: req.ModelName, IsActive: true, CreatedAt: now, UpdatedAt: now}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing provider returns 400",
			id:         modelID.String(),
			body:       UpdateAIModelRequest{ModelName: "gpt-4o"},
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			body: UpdateAIModelRequest{Provider: "openai", ModelName: "gpt-4o"},
			storeFn: func(_ context.Context, id uuid.UUID, _ UpdateAIModelRequest) (*AIModelRow, error) {
				return nil, fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{updateAIModelFn: tt.storeFn}
			r := newAITestRouter(store)

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/ai/models/"+tt.id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAIModelDelete(t *testing.T) {
	t.Parallel()
	modelID := uuid.New()

	tests := []struct {
		name       string
		id         string
		storeFn    func(ctx context.Context, id uuid.UUID) error
		wantStatus int
	}{
		{
			name:       "found returns 204",
			id:         modelID.String(),
			storeFn:    func(_ context.Context, _ uuid.UUID) error { return nil },
			wantStatus: http.StatusNoContent,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			storeFn: func(_ context.Context, id uuid.UUID) error {
				return fmt.Errorf("ai model %s: %w", id, domain.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{deleteAIModelFn: tt.storeFn}
			r := newAITestRouter(store)

			req := httptest.NewRequest(http.MethodDelete, "/ai/models/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAIBudgetCreate(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	budgetID := uuid.New()
	consumerID := uuid.New()

	tests := []struct {
		name       string
		body       any
		storeFn    func(ctx context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error)
		wantStatus int
	}{
		{
			name: "valid create returns 201",
			body: CreateTokenBudgetRequest{ConsumerID: &consumerID, Period: "day"},
			storeFn: func(_ context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error) {
				return &TokenBudgetRow{
					ID: budgetID, ConsumerID: req.ConsumerID, Period: req.Period,
					Enforcement: "hard", PeriodStart: now, CreatedAt: now, UpdatedAt: now,
				}, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid period returns 400",
			body:       CreateTokenBudgetRequest{ConsumerID: &consumerID, Period: "week"},
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing consumer and team returns 400",
			body:       CreateTokenBudgetRequest{Period: "day"},
			storeFn:    nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{createTokenBudgetFn: tt.storeFn}
			r := newAITestRouter(store)

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/ai/budgets", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				var got TokenBudgetRow
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, budgetID, got.ID)
			}
		})
	}
}

func TestAIBudgetList(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)

	store := &mockAIStore{
		listTokenBudgetsPagedFn: func(_ context.Context, _ ListParams) (*PagedResponse[TokenBudgetRow], error) {
			return &PagedResponse[TokenBudgetRow]{
				Data: []TokenBudgetRow{
					{ID: uuid.New(), Period: "day", Enforcement: "hard", PeriodStart: now, CreatedAt: now, UpdatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}
	r := newAITestRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/ai/budgets", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got PagedResponse[TokenBudgetRow]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Data, 1)
}

func TestAIBudgetUpdate(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	budgetID := uuid.New()
	consumerID := uuid.New()

	store := &mockAIStore{
		updateTokenBudgetFn: func(_ context.Context, id uuid.UUID, req UpdateTokenBudgetRequest) (*TokenBudgetRow, error) {
			return &TokenBudgetRow{
				ID: id, ConsumerID: req.ConsumerID, Period: req.Period,
				Enforcement: "hard", PeriodStart: now, CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}
	r := newAITestRouter(store)

	body, _ := json.Marshal(UpdateTokenBudgetRequest{ConsumerID: &consumerID, Period: "month"})
	req := httptest.NewRequest(http.MethodPut, "/ai/budgets/"+budgetID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAIBudgetDelete(t *testing.T) {
	t.Parallel()
	budgetID := uuid.New()

	tests := []struct {
		name       string
		id         string
		storeFn    func(ctx context.Context, id uuid.UUID) error
		wantStatus int
	}{
		{
			name:       "found returns 204",
			id:         budgetID.String(),
			storeFn:    func(_ context.Context, _ uuid.UUID) error { return nil },
			wantStatus: http.StatusNoContent,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			storeFn: func(_ context.Context, id uuid.UUID) error {
				return fmt.Errorf("token budget %s: %w", id, domain.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAIStore{deleteTokenBudgetFn: tt.storeFn}
			r := newAITestRouter(store)

			req := httptest.NewRequest(http.MethodDelete, "/ai/budgets/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
