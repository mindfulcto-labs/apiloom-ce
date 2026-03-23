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

// --- Mock stores for consumer handler tests ---

type mockConsumerConfigStore struct {
	createConsumerFunc func(ctx context.Context, name string, metadata json.RawMessage) (*domain.Consumer, error)
	getConsumerFunc    func(ctx context.Context, id uuid.UUID) (*domain.Consumer, error)
	deleteConsumerFunc func(ctx context.Context, id uuid.UUID) error
	createAPIKeyFunc   func(ctx context.Context, consumerID uuid.UUID, permissions json.RawMessage) (string, *domain.APIKey, error)
	revokeAPIKeyFunc   func(ctx context.Context, id uuid.UUID) error
	rotateAPIKeyFunc   func(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error)
	getAPIKeyFunc      func(ctx context.Context, id uuid.UUID) (*domain.APIKey, error)
}

func (m *mockConsumerConfigStore) CreateConsumer(ctx context.Context, name string, metadata json.RawMessage) (*domain.Consumer, error) {
	if m.createConsumerFunc != nil {
		return m.createConsumerFunc(ctx, name, metadata)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) GetConsumer(ctx context.Context, id uuid.UUID) (*domain.Consumer, error) {
	if m.getConsumerFunc != nil {
		return m.getConsumerFunc(ctx, id)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) DeleteConsumer(ctx context.Context, id uuid.UUID) error {
	if m.deleteConsumerFunc != nil {
		return m.deleteConsumerFunc(ctx, id)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) CreateAPIKey(ctx context.Context, consumerID uuid.UUID, permissions json.RawMessage) (string, *domain.APIKey, error) {
	if m.createAPIKeyFunc != nil {
		return m.createAPIKeyFunc(ctx, consumerID, permissions)
	}
	return "", nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	if m.revokeAPIKeyFunc != nil {
		return m.revokeAPIKeyFunc(ctx, id)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error) {
	if m.rotateAPIKeyFunc != nil {
		return m.rotateAPIKeyFunc(ctx, oldKeyID, gracePeriod)
	}
	return "", nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerConfigStore) GetAPIKey(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	if m.getAPIKeyFunc != nil {
		return m.getAPIKeyFunc(ctx, id)
	}
	return nil, fmt.Errorf("not implemented")
}

type mockConsumerStore struct {
	updateConsumerFunc    func(ctx context.Context, id uuid.UUID, req UpdateConsumerRequest) (*domain.Consumer, error)
	listConsumersPagedFn  func(ctx context.Context, params ListParams) (*PagedResponse[domain.Consumer], error)
	listAPIKeysFn         func(ctx context.Context, consumerID uuid.UUID, params ListParams) (*PagedResponse[domain.APIKey], error)
}

func (m *mockConsumerStore) UpdateConsumer(ctx context.Context, id uuid.UUID, req UpdateConsumerRequest) (*domain.Consumer, error) {
	if m.updateConsumerFunc != nil {
		return m.updateConsumerFunc(ctx, id, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerStore) ListConsumersPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.Consumer], error) {
	if m.listConsumersPagedFn != nil {
		return m.listConsumersPagedFn(ctx, params)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockConsumerStore) ListAPIKeys(ctx context.Context, consumerID uuid.UUID, params ListParams) (*PagedResponse[domain.APIKey], error) {
	if m.listAPIKeysFn != nil {
		return m.listAPIKeysFn(ctx, consumerID, params)
	}
	return nil, fmt.Errorf("not implemented")
}

func newTestConsumerHandler(cfgStore ConsumerConfigStore, store ConsumerStore) *ConsumerHandler {
	return &ConsumerHandler{
		configStore: cfgStore,
		store:       store,
	}
}

func TestConsumerHandler_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	consumerID := uuid.New()

	tests := []struct {
		name       string
		body       string
		cfgStore   *mockConsumerConfigStore
		wantStatus int
	}{
		{
			name: "valid consumer returns 201",
			body: `{"name":"test-consumer"}`,
			cfgStore: &mockConsumerConfigStore{
				createConsumerFunc: func(_ context.Context, name string, metadata json.RawMessage) (*domain.Consumer, error) {
					return &domain.Consumer{
						ID: consumerID, Name: name, Metadata: metadata,
						IsActive: true, CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "short name returns 400",
			body:       `{"name":"ab"}`,
			cfgStore:   &mockConsumerConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate returns 409",
			body: `{"name":"dup-consumer"}`,
			cfgStore: &mockConsumerConfigStore{
				createConsumerFunc: func(_ context.Context, name string, _ json.RawMessage) (*domain.Consumer, error) {
					return nil, fmt.Errorf("consumer name %q: %w", name, domain.ErrAlreadyExists)
				},
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestConsumerHandler(tt.cfgStore, &mockConsumerStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				var resp ConsumerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, consumerID, resp.ID)
			}
		})
	}
}

func TestConsumerHandler_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	consumerID := uuid.New()

	tests := []struct {
		name       string
		id         string
		cfgStore   *mockConsumerConfigStore
		wantStatus int
	}{
		{
			name: "existing returns 200",
			id:   consumerID.String(),
			cfgStore: &mockConsumerConfigStore{
				getConsumerFunc: func(_ context.Context, id uuid.UUID) (*domain.Consumer, error) {
					return &domain.Consumer{
						ID: id, Name: "test", IsActive: true,
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "bad UUID returns 400",
			id:         "bad",
			cfgStore:   &mockConsumerConfigStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found returns 404",
			id:   uuid.New().String(),
			cfgStore: &mockConsumerConfigStore{
				getConsumerFunc: func(_ context.Context, id uuid.UUID) (*domain.Consumer, error) {
					return nil, fmt.Errorf("consumer %s: %w", id, domain.ErrNotFound)
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestConsumerHandler(tt.cfgStore, &mockConsumerStore{})
			r := chi.NewRouter()
			h.Routes(r)

			req := httptest.NewRequest(http.MethodGet, "/"+tt.id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestConsumerHandler_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	store := &mockConsumerStore{
		listConsumersPagedFn: func(_ context.Context, _ ListParams) (*PagedResponse[domain.Consumer], error) {
			return &PagedResponse[domain.Consumer]{
				Data: []domain.Consumer{
					{ID: uuid.New(), Name: "c1", IsActive: true, CreatedAt: now, UpdatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}

	h := newTestConsumerHandler(&mockConsumerConfigStore{}, store)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[ConsumerResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
}

func TestConsumerHandler_Update(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	consumerID := uuid.New()

	store := &mockConsumerStore{
		updateConsumerFunc: func(_ context.Context, id uuid.UUID, req UpdateConsumerRequest) (*domain.Consumer, error) {
			return &domain.Consumer{
				ID: id, Name: req.Name, IsActive: true,
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}

	h := newTestConsumerHandler(&mockConsumerConfigStore{}, store)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPut, "/"+consumerID.String(), bytes.NewBufferString(`{"name":"updated-consumer"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestConsumerHandler_Delete(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()

	cfgStore := &mockConsumerConfigStore{
		deleteConsumerFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}

	h := newTestConsumerHandler(cfgStore, &mockConsumerStore{})
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodDelete, "/"+consumerID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestConsumerHandler_CreateAPIKey(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()
	keyID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	cfgStore := &mockConsumerConfigStore{
		getConsumerFunc: func(_ context.Context, id uuid.UUID) (*domain.Consumer, error) {
			return &domain.Consumer{ID: id, Name: "test", IsActive: true}, nil
		},
		createAPIKeyFunc: func(_ context.Context, cID uuid.UUID, _ json.RawMessage) (string, *domain.APIKey, error) {
			return "abcdef01234567890123456789abcdef01234567890123456789abcdef012345", &domain.APIKey{
				ID: keyID, ConsumerID: cID,
				KeyPrefix: "abcdef01", IsActive: true,
				CreatedAt: now,
			}, nil
		},
	}

	h := newTestConsumerHandler(cfgStore, &mockConsumerStore{})
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/"+consumerID.String()+"/keys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "key")
	assert.Contains(t, resp, "api_key")
	// Raw key should be present
	rawKey, ok := resp["key"].(string)
	require.True(t, ok)
	assert.Len(t, rawKey, 64)
	// key_hash must NOT be in api_key response
	apiKeyMap, ok := resp["api_key"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, apiKeyMap, "key_hash")
}

func TestConsumerHandler_ListAPIKeys(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	cfgStore := &mockConsumerConfigStore{
		getConsumerFunc: func(_ context.Context, id uuid.UUID) (*domain.Consumer, error) {
			return &domain.Consumer{ID: id, Name: "test", IsActive: true}, nil
		},
	}
	store := &mockConsumerStore{
		listAPIKeysFn: func(_ context.Context, cID uuid.UUID, _ ListParams) (*PagedResponse[domain.APIKey], error) {
			return &PagedResponse[domain.APIKey]{
				Data: []domain.APIKey{
					{ID: uuid.New(), ConsumerID: cID, KeyPrefix: "abc12345", IsActive: true, CreatedAt: now},
				},
				HasMore: false,
			}, nil
		},
	}

	h := newTestConsumerHandler(cfgStore, store)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/"+consumerID.String()+"/keys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[APIKeyResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
}

func TestConsumerHandler_RotateAPIKey(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()
	oldKeyID := uuid.New()
	newKeyID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	cfgStore := &mockConsumerConfigStore{
		getAPIKeyFunc: func(_ context.Context, id uuid.UUID) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: oldKeyID, ConsumerID: consumerID,
				IsActive: true, CreatedAt: now,
			}, nil
		},
		rotateAPIKeyFunc: func(_ context.Context, _ uuid.UUID, _ time.Duration) (string, *domain.APIKey, error) {
			return "newkey01234567890123456789abcdef01234567890123456789abcdef012345", &domain.APIKey{
				ID: newKeyID, ConsumerID: consumerID,
				KeyPrefix: "newkey01", IsActive: true,
				CreatedAt: now,
			}, nil
		},
	}

	h := newTestConsumerHandler(cfgStore, &mockConsumerStore{})
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodPost, "/"+consumerID.String()+"/keys/"+oldKeyID.String()+"/rotate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "key")
}

func TestConsumerHandler_RevokeAPIKey(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()
	keyID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	cfgStore := &mockConsumerConfigStore{
		getAPIKeyFunc: func(_ context.Context, _ uuid.UUID) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: keyID, ConsumerID: consumerID,
				IsActive: true, CreatedAt: now,
			}, nil
		},
		revokeAPIKeyFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}

	h := newTestConsumerHandler(cfgStore, &mockConsumerStore{})
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodDelete, "/"+consumerID.String()+"/keys/"+keyID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestConsumerHandler_BOLAPrevention(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()
	otherConsumerID := uuid.New()
	keyID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Key belongs to otherConsumerID, but URL says consumerID
	cfgStore := &mockConsumerConfigStore{
		getAPIKeyFunc: func(_ context.Context, _ uuid.UUID) (*domain.APIKey, error) {
			return &domain.APIKey{
				ID: keyID, ConsumerID: otherConsumerID,
				IsActive: true, CreatedAt: now,
			}, nil
		},
	}

	h := newTestConsumerHandler(cfgStore, &mockConsumerStore{})
	r := chi.NewRouter()
	h.Routes(r)

	// Try to rotate a key that belongs to a different consumer
	req := httptest.NewRequest(http.MethodPost, "/"+consumerID.String()+"/keys/"+keyID.String()+"/rotate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should return 404 (not 403) to avoid leaking existence
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}
