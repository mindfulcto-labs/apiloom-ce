package observability

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric instruments for the gateway.
type Metrics struct {
	// RequestsTotal counts inbound HTTP requests by route, method, status.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration observes request latency by route, method, status.
	RequestDuration *prometheus.HistogramVec

	// ActiveConnections tracks currently in-flight requests.
	ActiveConnections prometheus.Gauge

	// UpstreamRequestsTotal counts outbound upstream HTTP requests.
	UpstreamRequestsTotal *prometheus.CounterVec

	// UpstreamRequestDuration observes upstream request latency.
	UpstreamRequestDuration *prometheus.HistogramVec

	// ConfigReloadTotal counts config reload attempts by status.
	ConfigReloadTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all gateway Prometheus metrics on the given registerer.
// It also registers Go runtime collectors (goroutines, memory, GC).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_requests_total",
				Help: "Total number of HTTP requests processed.",
			},
			[]string{"route", "method", "status_code"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "flexgw_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"route", "method", "status_code"},
		),
		ActiveConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "flexgw_active_connections",
				Help: "Number of currently active connections.",
			},
		),
		UpstreamRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_upstream_requests_total",
				Help: "Total number of upstream HTTP requests.",
			},
			[]string{"upstream", "method", "status_code"},
		),
		UpstreamRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "flexgw_upstream_request_duration_seconds",
				Help:    "Upstream HTTP request duration in seconds.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"upstream", "method", "status_code"},
		),
		ConfigReloadTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_config_reload_total",
				Help: "Total number of configuration reload attempts.",
			},
			[]string{"status"},
		),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.ActiveConnections,
		m.UpstreamRequestsTotal,
		m.UpstreamRequestDuration,
		m.ConfigReloadTotal,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

// Middleware returns HTTP middleware that records per-request Prometheus metrics.
// The "route" label uses the chi route pattern (not raw path) for cardinality control.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.ActiveConnections.Inc()
		defer m.ActiveConnections.Dec()

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(sw, r)

		duration := time.Since(start).Seconds()

		// Use chi route pattern for cardinality control, fall back to path.
		route := r.URL.Path
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				route = pattern
			}
		}

		status := statusCodeStr(sw.statusCode)
		method := r.Method

		m.RequestsTotal.WithLabelValues(route, method, status).Inc()
		m.RequestDuration.WithLabelValues(route, method, status).Observe(duration)
	})
}

// MetricsHandler returns an http.Handler that serves the /metrics endpoint
// in Prometheus exposition format.
func MetricsHandler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
