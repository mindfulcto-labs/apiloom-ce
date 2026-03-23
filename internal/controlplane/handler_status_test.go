package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStatusDB implements StatusDB for unit testing.
type mockStatusDB struct {
	pingFn  func(ctx context.Context) error
	countFn func(ctx context.Context, table string) (int64, error)
}

func (m *mockStatusDB) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}

func (m *mockStatusDB) CountActive(ctx context.Context, table string) (int64, error) {
	if m.countFn != nil {
		return m.countFn(ctx, table)
	}
	return 0, nil
}

func newStatusTestRouter(db StatusDB) *chi.Mux {
	h := NewStatusHandler(db, "v0.1.0-test", time.Now().Add(-1*time.Hour))
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func TestStatusHandler_Healthy(t *testing.T) {
	t.Parallel()

	db := &mockStatusDB{
		pingFn: func(_ context.Context) error { return nil },
		countFn: func(_ context.Context, table string) (int64, error) {
			switch table {
			case "routes":
				return 5, nil
			case "backends":
				return 3, nil
			case "consumers":
				return 10, nil
			default:
				return 0, nil
			}
		},
	}
	r := newStatusTestRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v0.1.0-test", resp.Version)
	assert.Greater(t, resp.UptimeSeconds, float64(0))
	assert.Equal(t, "healthy", resp.Database.Status)
	assert.GreaterOrEqual(t, resp.Database.LatencyMS, float64(0))
	assert.Equal(t, int64(5), resp.Resources.Routes)
	assert.Equal(t, int64(3), resp.Resources.Backends)
	assert.Equal(t, int64(10), resp.Resources.Consumers)
}

func TestStatusHandler_DBDown(t *testing.T) {
	t.Parallel()

	db := &mockStatusDB{
		pingFn: func(_ context.Context) error { return errors.New("connection refused") },
	}
	r := newStatusTestRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Status endpoint still returns 200 even when DB is down.
	assert.Equal(t, http.StatusOK, w.Code)

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Database.Status)
	assert.Equal(t, int64(0), resp.Resources.Routes)
	assert.Equal(t, int64(0), resp.Resources.Backends)
	assert.Equal(t, int64(0), resp.Resources.Consumers)
}

func TestStatusHandler_VersionAndUptime(t *testing.T) {
	t.Parallel()

	startTime := time.Now().Add(-30 * time.Minute)
	db := &mockStatusDB{}
	h := NewStatusHandler(db, "dev", startTime)
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "dev", resp.Version)
	// At least 29 minutes of uptime (allowing 1 minute for test execution)
	assert.Greater(t, resp.UptimeSeconds, float64(29*60))
}
