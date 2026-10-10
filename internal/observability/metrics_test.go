package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	require.NotNil(t, m)

	// Verify all metrics are registered by gathering them.
	families, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	// Go runtime metrics should be present.
	assert.True(t, names["go_goroutines"], "expected go_goroutines metric")
}

func TestNewMetrics_RegisteredNames(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	require.NotNil(t, m)

	// Observe/increment all metric types to ensure they appear in gather.
	m.RequestsTotal.WithLabelValues("/test", "GET", "200").Inc()
	m.RequestDuration.WithLabelValues("/test", "GET", "200").Observe(0.1)
	m.UpstreamRequestsTotal.WithLabelValues("backend-1", "GET", "200").Inc()
	m.UpstreamRequestDuration.WithLabelValues("backend-1", "GET", "200").Observe(0.05)
	m.ConfigReloadTotal.WithLabelValues("success").Inc()

	families, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	expectedMetrics := []string{
		"flexgw_requests_total",
		"flexgw_request_duration_seconds",
		"flexgw_active_connections",
		"flexgw_upstream_requests_total",
		"flexgw_upstream_request_duration_seconds",
		"flexgw_config_reload_total",
	}

	for _, name := range expectedMetrics {
		assert.True(t, names[name], "expected metric %q to be registered", name)
	}
}

func TestMetricsMiddleware(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := m.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Gather and check that request counter was incremented.
	families, err := reg.Gather()
	require.NoError(t, err)

	var foundRequestTotal bool
	for _, f := range families {
		if f.GetName() == "flexgw_requests_total" {
			foundRequestTotal = true
			require.NotEmpty(t, f.GetMetric())
			metric := f.GetMetric()[0]
			assert.Equal(t, float64(1), metric.GetCounter().GetValue())
		}
	}
	assert.True(t, foundRequestTotal, "flexgw_requests_total should be present after request")
}

func TestMetricsMiddleware_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "200 OK", statusCode: http.StatusOK},
		{name: "404 Not Found", statusCode: http.StatusNotFound},
		{name: "500 Internal Server Error", statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			m := NewMetrics(reg)

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			handler := m.Middleware(inner)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.statusCode, rec.Code)
		})
	}
}

func TestMetricsHandler(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	// Generate some metrics.
	m.RequestsTotal.WithLabelValues("/test", "GET", "200").Inc()

	handler := MetricsHandler(reg)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.True(t, strings.Contains(body, "flexgw_requests_total"), "expected flexgw_requests_total in /metrics output")
}
