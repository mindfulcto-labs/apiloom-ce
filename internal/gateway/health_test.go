package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// mockPingChecker implements PingChecker for tests.
type mockPingChecker struct {
	err   error
	delay time.Duration
}

func (m *mockPingChecker) Ping(ctx context.Context) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.err
}

func TestHealthLiveness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     *domain.GatewayConfig
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 200 with nil config",
			config:     nil,
			pingErr:    nil,
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "returns 200 with loaded config",
			config:     &domain.GatewayConfig{Version: 1},
			pingErr:    nil,
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "returns 200 when postgres is down",
			config:     &domain.GatewayConfig{Version: 1},
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewHealthHandler(
				&mockConfigProvider{config: tt.config},
				&mockPingChecker{err: tt.pingErr},
			)

			mux := http.NewServeMux()
			h.Register(mux)

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var resp HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantBody, resp.Status)
			assert.False(t, resp.Timestamp.IsZero())
		})
	}
}

func TestHealthReadiness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      *domain.GatewayConfig
		pingErr     error
		wantStatus  int
		wantBody    string
		wantCheckFn func(t *testing.T, resp HealthResponse)
	}{
		{
			name:       "returns 200 when config loaded and postgres healthy",
			config:     &domain.GatewayConfig{Version: 1},
			pingErr:    nil,
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "returns 503 when config not loaded",
			config:     nil,
			pingErr:    nil,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "failing",
			wantCheckFn: func(t *testing.T, resp HealthResponse) {
				t.Helper()
				configCheck, ok := resp.Checks["config"]
				require.True(t, ok, "config check should be present")
				assert.Equal(t, "failing", configCheck.Status)
			},
		},
		{
			name:       "returns 503 when postgres ping fails",
			config:     &domain.GatewayConfig{Version: 1},
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "failing",
			wantCheckFn: func(t *testing.T, resp HealthResponse) {
				t.Helper()
				pgCheck, ok := resp.Checks["postgres"]
				require.True(t, ok, "postgres check should be present")
				assert.Equal(t, "failing", pgCheck.Status)
				assert.NotEmpty(t, pgCheck.Error)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewHealthHandler(
				&mockConfigProvider{config: tt.config},
				&mockPingChecker{err: tt.pingErr},
			)

			mux := http.NewServeMux()
			h.Register(mux)

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var resp HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantBody, resp.Status)

			if tt.wantCheckFn != nil {
				tt.wantCheckFn(t, resp)
			}
		})
	}
}

func TestHealthDeep(t *testing.T) {
	t.Parallel()

	t.Run("returns goroutine count and memory stats", func(t *testing.T) {
		t.Parallel()
		h := NewHealthHandler(
			&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
			&mockPingChecker{err: nil},
		)

		mux := http.NewServeMux()
		h.Register(mux)

		req := httptest.NewRequest(http.MethodGet, "/healthz/deep", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var resp HealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "ok", resp.Status)

		// Check goroutine count is present
		goroutineCheck, ok := resp.Checks["goroutines"]
		require.True(t, ok, "goroutines check should be present")
		assert.Equal(t, "ok", goroutineCheck.Status)
		assert.NotNil(t, goroutineCheck.Value)

		// Check memory stats are present
		memCheck, ok := resp.Checks["memory"]
		require.True(t, ok, "memory check should be present")
		assert.Equal(t, "ok", memCheck.Status)
		assert.NotNil(t, memCheck.Value)

		// Check postgres latency is present
		pgCheck, ok := resp.Checks["postgres"]
		require.True(t, ok, "postgres check should be present")
		assert.Equal(t, "ok", pgCheck.Status)
		assert.NotEmpty(t, pgCheck.Latency)
	})

	t.Run("returns degraded when postgres latency exceeds 500ms", func(t *testing.T) {
		t.Parallel()
		h := NewHealthHandler(
			&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
			&mockPingChecker{err: nil, delay: 600 * time.Millisecond},
		)

		mux := http.NewServeMux()
		h.Register(mux)

		req := httptest.NewRequest(http.MethodGet, "/healthz/deep", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp HealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "degraded", resp.Status)
	})

	t.Run("returns ok with failing postgres check when ping fails", func(t *testing.T) {
		t.Parallel()
		h := NewHealthHandler(
			&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
			&mockPingChecker{err: errors.New("connection refused")},
		)

		mux := http.NewServeMux()
		h.Register(mux)

		req := httptest.NewRequest(http.MethodGet, "/healthz/deep", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Deep health still returns 200 — it reports status, doesn't gate on it
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp HealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		pgCheck, ok := resp.Checks["postgres"]
		require.True(t, ok)
		assert.Equal(t, "failing", pgCheck.Status)
		assert.NotEmpty(t, pgCheck.Error)
	})
}

func TestHealthContentType(t *testing.T) {
	t.Parallel()

	h := NewHealthHandler(
		&mockConfigProvider{config: &domain.GatewayConfig{Version: 1}},
		&mockPingChecker{err: nil},
	)

	mux := http.NewServeMux()
	h.Register(mux)

	endpoints := []string{"/healthz", "/readyz", "/healthz/deep"}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		})
	}
}
