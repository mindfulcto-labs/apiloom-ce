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

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/plugin"
)

// mockPluginStoreCP implements PluginStoreInterface for handler unit testing.
type mockPluginStoreCP struct {
	createFn        func(ctx context.Context, rec *plugin.PluginRecord) (*plugin.PluginRecord, error)
	getFn           func(ctx context.Context, id uuid.UUID) (*plugin.PluginRecord, error)
	listFn          func(ctx context.Context, params plugin.ListPluginParams) (*plugin.PagedPlugins, error)
	updateConfigFn  func(ctx context.Context, id uuid.UUID, config json.RawMessage) (*plugin.PluginRecord, error)
	updateEnabledFn func(ctx context.Context, id uuid.UUID, enabled bool) (*plugin.PluginRecord, error)
	updateOrderFn   func(ctx context.Context, id uuid.UUID, ordering int) (*plugin.PluginRecord, error)
	deleteFn        func(ctx context.Context, id uuid.UUID) error
}

func (m *mockPluginStoreCP) Create(ctx context.Context, rec *plugin.PluginRecord) (*plugin.PluginRecord, error) {
	return m.createFn(ctx, rec)
}
func (m *mockPluginStoreCP) Get(ctx context.Context, id uuid.UUID) (*plugin.PluginRecord, error) {
	return m.getFn(ctx, id)
}
func (m *mockPluginStoreCP) List(ctx context.Context, params plugin.ListPluginParams) (*plugin.PagedPlugins, error) {
	return m.listFn(ctx, params)
}
func (m *mockPluginStoreCP) UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) (*plugin.PluginRecord, error) {
	return m.updateConfigFn(ctx, id, config)
}
func (m *mockPluginStoreCP) UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*plugin.PluginRecord, error) {
	return m.updateEnabledFn(ctx, id, enabled)
}
func (m *mockPluginStoreCP) UpdateOrdering(ctx context.Context, id uuid.UUID, ordering int) (*plugin.PluginRecord, error) {
	return m.updateOrderFn(ctx, id, ordering)
}
func (m *mockPluginStoreCP) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

func newPluginTestRouter(store PluginStoreInterface) *chi.Mux {
	r := chi.NewRouter()
	RegisterPluginRoutes(r, store)
	return r
}

func samplePluginRecord() *plugin.PluginRecord {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &plugin.PluginRecord{
		ID:         uuid.New(),
		Name:       "test-plugin",
		PluginType: "native",
		Config:     json.RawMessage(`{"key":"value"}`),
		Target:     "global",
		Ordering:   10,
		IsEnabled:  false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestPluginCreate(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	tests := []struct {
		name       string
		body       any
		storeFn    func(ctx context.Context, r *plugin.PluginRecord) (*plugin.PluginRecord, error)
		wantStatus int
	}{
		{
			name: "success",
			body: CreatePluginRequest{Name: "test-plugin", PluginType: "native", Config: json.RawMessage(`{"key":"value"}`)},
			storeFn: func(_ context.Context, _ *plugin.PluginRecord) (*plugin.PluginRecord, error) {
				return rec, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name",
			body:       CreatePluginRequest{PluginType: "native"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid type",
			body:       CreatePluginRequest{Name: "test", PluginType: "wasm"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "grpc without address",
			body:       CreatePluginRequest{Name: "test", PluginType: "grpc"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "conflict",
			body: CreatePluginRequest{Name: "test-plugin", PluginType: "native"},
			storeFn: func(_ context.Context, _ *plugin.PluginRecord) (*plugin.PluginRecord, error) {
				return nil, fmt.Errorf("plugin test-plugin: %w", domain.ErrAlreadyExists)
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockPluginStoreCP{
				createFn: tt.storeFn,
			}
			router := newPluginTestRouter(store)

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/plugins", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusCreated {
				var got plugin.PluginRecord
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, rec.Name, got.Name)
			}
		})
	}
}

func TestPluginList(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	store := &mockPluginStoreCP{
		listFn: func(_ context.Context, _ plugin.ListPluginParams) (*plugin.PagedPlugins, error) {
			return &plugin.PagedPlugins{
				Data:    []plugin.PluginRecord{*rec},
				HasMore: false,
			}, nil
		},
	}
	router := newPluginTestRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/plugins?limit=10&enabled=true&type=native", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got plugin.PagedPlugins
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Data, 1)
}

func TestPluginGetByID(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	tests := []struct {
		name       string
		id         string
		storeFn    func(ctx context.Context, id uuid.UUID) (*plugin.PluginRecord, error)
		wantStatus int
	}{
		{
			name: "found",
			id:   rec.ID.String(),
			storeFn: func(_ context.Context, _ uuid.UUID) (*plugin.PluginRecord, error) {
				return rec, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid id",
			id:         "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found",
			id:   uuid.New().String(),
			storeFn: func(_ context.Context, id uuid.UUID) (*plugin.PluginRecord, error) {
				return nil, fmt.Errorf("plugin %s: %w", id, domain.ErrNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockPluginStoreCP{
				getFn: tt.storeFn,
			}
			router := newPluginTestRouter(store)

			req := httptest.NewRequest(http.MethodGet, "/plugins/"+tt.id, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestPluginUpdate(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	store := &mockPluginStoreCP{
		updateConfigFn: func(_ context.Context, _ uuid.UUID, cfg json.RawMessage) (*plugin.PluginRecord, error) {
			updated := *rec
			updated.Config = cfg
			return &updated, nil
		},
	}
	router := newPluginTestRouter(store)

	body, _ := json.Marshal(UpdatePluginRequest{Config: json.RawMessage(`{"new": true}`)})
	req := httptest.NewRequest(http.MethodPatch, "/plugins/"+rec.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginDelete(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	store := &mockPluginStoreCP{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}
	router := newPluginTestRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/plugins/"+rec.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestPluginEnable(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	store := &mockPluginStoreCP{
		updateEnabledFn: func(_ context.Context, _ uuid.UUID, enabled bool) (*plugin.PluginRecord, error) {
			updated := *rec
			updated.IsEnabled = enabled
			return &updated, nil
		},
	}
	router := newPluginTestRouter(store)

	req := httptest.NewRequest(http.MethodPatch, "/plugins/"+rec.ID.String()+"/enable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got plugin.PluginRecord
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.IsEnabled)
}

func TestPluginDisable(t *testing.T) {
	t.Parallel()
	rec := samplePluginRecord()

	store := &mockPluginStoreCP{
		updateEnabledFn: func(_ context.Context, _ uuid.UUID, enabled bool) (*plugin.PluginRecord, error) {
			updated := *rec
			updated.IsEnabled = enabled
			return &updated, nil
		},
	}
	router := newPluginTestRouter(store)

	req := httptest.NewRequest(http.MethodPatch, "/plugins/"+rec.ID.String()+"/disable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got plugin.PluginRecord
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.IsEnabled)
}
