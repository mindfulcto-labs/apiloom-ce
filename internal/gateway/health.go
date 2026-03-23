package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// PingChecker checks connectivity to an external dependency.
// Satisfied by pgxpool.Pool.
type PingChecker interface {
	Ping(ctx context.Context) error
}

// HealthResponse is the JSON response from health endpoints.
type HealthResponse struct {
	Status    string           `json:"status"`
	Checks   map[string]Check `json:"checks,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// Check represents a single health check result.
type Check struct {
	Status  string `json:"status"`
	Latency string `json:"latency_ms,omitempty"`
	Error   string `json:"error,omitempty"`
	Value   any    `json:"value,omitempty"`
}

// MemoryStats holds runtime memory information for the deep health check.
type MemoryStats struct {
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64 `json:"heap_sys_bytes"`
	NumGC          uint32 `json:"num_gc"`
}

// HealthHandler provides health check endpoints.
// It registers on a separate http.ServeMux, NOT in the Lura pipeline.
type HealthHandler struct {
	configLoader ConfigProvider
	pool         PingChecker
}

// NewHealthHandler creates a HealthHandler with the given config loader and ping checker.
func NewHealthHandler(loader ConfigProvider, pool PingChecker) *HealthHandler {
	return &HealthHandler{
		configLoader: loader,
		pool:         pool,
	}
}

// Register adds health endpoints to the given ServeMux.
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.HandleLiveness)
	mux.HandleFunc("/readyz", h.HandleReadiness)
	mux.HandleFunc("/healthz/deep", h.HandleDeep)
}

// HandleLiveness always returns 200 OK. MUST NOT check Postgres or any external dependency.
// This prevents Kubernetes from cascading restarts during dependency outages.
func (h *HealthHandler) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	})
}

// HandleReadiness checks that config is loaded AND Postgres is reachable.
// Returns 200 when ready, 503 when not.
func (h *HealthHandler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]Check)
	status := "ok"
	httpStatus := http.StatusOK

	// Check config loaded
	if h.configLoader.Get() == nil {
		checks["config"] = Check{Status: "failing", Error: "config not loaded"}
		status = "failing"
		httpStatus = http.StatusServiceUnavailable
	} else {
		checks["config"] = Check{Status: "ok"}
	}

	// Check Postgres connectivity
	if h.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := h.pool.Ping(ctx); err != nil {
			checks["postgres"] = Check{Status: "failing", Error: err.Error()}
			status = "failing"
			httpStatus = http.StatusServiceUnavailable
		} else {
			checks["postgres"] = Check{Status: "ok"}
		}
	}

	writeJSON(w, httpStatus, HealthResponse{
		Status:    status,
		Checks:   checks,
		Timestamp: time.Now(),
	})
}

// HandleDeep returns detailed health information including Postgres latency,
// goroutine count, and memory stats. Always returns 200 -- status field
// indicates "ok", "degraded", or "failing".
func (h *HealthHandler) HandleDeep(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]Check)
	overallStatus := "ok"

	// Postgres latency
	if h.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		start := time.Now()
		err := h.pool.Ping(ctx)
		latency := time.Since(start)

		if err != nil {
			checks["postgres"] = Check{
				Status:  "failing",
				Latency: fmt.Sprintf("%.2f", float64(latency.Milliseconds())),
				Error:   err.Error(),
			}
			overallStatus = "failing"
		} else if latency > 500*time.Millisecond {
			checks["postgres"] = Check{
				Status:  "degraded",
				Latency: fmt.Sprintf("%.2f", float64(latency.Milliseconds())),
			}
			overallStatus = "degraded"
		} else {
			checks["postgres"] = Check{
				Status:  "ok",
				Latency: fmt.Sprintf("%.2f", float64(latency.Milliseconds())),
			}
		}
	}

	// Goroutine count
	numGoroutines := runtime.NumGoroutine()
	checks["goroutines"] = Check{
		Status: "ok",
		Value:  numGoroutines,
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	checks["memory"] = Check{
		Status: "ok",
		Value: MemoryStats{
			HeapAllocBytes: memStats.HeapAlloc,
			HeapSysBytes:   memStats.HeapSys,
			NumGC:          memStats.NumGC,
		},
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    overallStatus,
		Checks:   checks,
		Timestamp: time.Now(),
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
