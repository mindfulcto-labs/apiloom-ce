package controlplane

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// StatusDB abstracts database operations needed by the status handler.
// Full implementation in 06-04 plan.
type StatusDB interface {
	Ping(ctx context.Context) error
	CountActive(ctx context.Context, table string) (int64, error)
}

// StatusResponse is the API response for the status endpoint.
type StatusResponse struct {
	Version       string          `json:"version"`
	UptimeSeconds float64         `json:"uptime_seconds"`
	Database      DatabaseStatus  `json:"database"`
	Resources     ResourceCounts  `json:"resources"`
}

// DatabaseStatus represents the database health check result.
type DatabaseStatus struct {
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
}

// ResourceCounts holds active resource counts.
type ResourceCounts struct {
	Routes    int64 `json:"routes"`
	Backends  int64 `json:"backends"`
	Consumers int64 `json:"consumers"`
}

// StatusHandler provides the /status endpoint.
type StatusHandler struct {
	db        StatusDB
	version   string
	startTime time.Time
}

// NewStatusHandler creates a StatusHandler.
func NewStatusHandler(db StatusDB, version string, startTime time.Time) *StatusHandler {
	return &StatusHandler{db: db, version: version, startTime: startTime}
}

// Routes mounts the status endpoint.
func (h *StatusHandler) Routes(r chi.Router) {
	r.Get("/status", h.status)
}

func (h *StatusHandler) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := StatusResponse{
		Version:       h.version,
		UptimeSeconds: time.Since(h.startTime).Seconds(),
	}

	start := time.Now()
	if err := h.db.Ping(ctx); err != nil {
		resp.Database = DatabaseStatus{Status: "unhealthy", LatencyMS: float64(time.Since(start).Milliseconds())}
	} else {
		resp.Database = DatabaseStatus{
			Status:    "healthy",
			LatencyMS: float64(time.Since(start).Milliseconds()),
		}

		if n, err := h.db.CountActive(ctx, "routes"); err == nil {
			resp.Resources.Routes = n
		}
		if n, err := h.db.CountActive(ctx, "backends"); err == nil {
			resp.Resources.Backends = n
		}
		if n, err := h.db.CountActive(ctx, "consumers"); err == nil {
			resp.Resources.Consumers = n
		}
	}

	WriteJSON(w, http.StatusOK, resp)
}
