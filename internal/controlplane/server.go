// Package controlplane provides the control plane REST API server
// that manages routes, backends, consumers, AI models, and token budgets.
package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cto-venkat/apiloom-ce/internal/config"
	"github.com/cto-venkat/apiloom-ce/internal/plugin"
)

// Server holds the control plane API dependencies and wires all handlers
// into a single HTTP handler.
type Server struct {
	pool        *pgxpool.Pool
	configStore *config.ConfigStore
	store       *Store
	version     string
	startTime   time.Time
}

// NewServer creates a control plane Server. The pool may be nil for testing
// (handlers that need DB access will return errors).
func NewServer(pool *pgxpool.Pool, version string, startTime time.Time) *Server {
	s := &Server{
		pool:      pool,
		version:   version,
		startTime: startTime,
	}
	if pool != nil {
		s.configStore = config.NewConfigStore(pool)
		s.store = NewStore(pool)
	}
	return s
}

// Handler returns an http.Handler with the chi router mounting all
// control plane endpoints under /api/v1/.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health endpoint (no auth required -- used by K8s probes).
	r.Get("/healthz", s.handleHealthz)

	r.Route("/api/v1", func(r chi.Router) {
		// All endpoints require org context.
		r.Group(func(r chi.Router) {
			r.Use(OrgContextMiddleware)

			// Status endpoint.
			statusDB := s.newStatusDB()
			statusHandler := NewStatusHandler(statusDB, s.version, s.startTime)
			statusHandler.Routes(r)

			// Route CRUD.
			routeHandler := s.newRouteHandler()
			r.Route("/routes", func(r chi.Router) {
				routeHandler.Routes(r)

				// API version history + canary config (nested under route).
				apiVersionHandler := s.newAPIVersionHandler()
				r.Route("/{id}/versions", apiVersionHandler.Routes)
				r.Route("/{id}/canary", apiVersionHandler.CanaryRoutes)
			})

			// Backend CRUD.
			backendHandler := s.newBackendHandler()
			r.Route("/backends", backendHandler.Routes)

			// Consumer & API Key CRUD.
			consumerHandler := s.newConsumerHandler()
			r.Route("/consumers", consumerHandler.Routes)

			// AI model and token budget CRUD.
			aiHandler := s.newAIHandler()
			aiHandler.Routes(r)

			// Config version history and rollback.
			configVersionHandler := s.newConfigVersionHandler()
			r.Route("/config", configVersionHandler.Routes)

			// Environment management.
			envHandler := s.newEnvironmentHandler()
			r.Route("/environments", envHandler.Routes)

			// Plugin CRUD.
			RegisterPluginRoutes(r, s.newPluginStore())
		})
	})

	return r
}

// handleHealthz returns JSON health status with postgres dependency probe.
// Returns 200 if healthy, 503 if postgres is down.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	type depStatus struct {
		Status  string `json:"status"`
		Latency string `json:"latency_ms,omitempty"`
		Detail  string `json:"detail,omitempty"`
	}
	type healthResp struct {
		Status       string               `json:"status"`
		Dependencies map[string]depStatus `json:"dependencies"`
	}

	deps := make(map[string]depStatus)
	overall := "ok"

	if s.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		start := time.Now()
		_, err := s.pool.Exec(ctx, "SELECT 1")
		latency := time.Since(start)
		if err != nil {
			deps["postgres"] = depStatus{Status: "down", Detail: "connection failed"}
			overall = "down"
		} else {
			deps["postgres"] = depStatus{
				Status:  "ok",
				Latency: fmt.Sprintf("%d", latency.Milliseconds()),
			}
		}
	}

	deps["plugins"] = depStatus{Status: "ok"}

	resp := healthResp{Status: overall, Dependencies: deps}
	w.Header().Set("Content-Type", "application/json")
	if overall == "down" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// OrgContextMiddleware extracts the organization ID from the X-Organization-ID header
// and injects it into the request context. Returns 400 if the header is missing or invalid.
func OrgContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgHeader := r.Header.Get("X-Organization-ID")
		if orgHeader == "" {
			http.Error(w, `{"error":"X-Organization-ID header is required"}`, http.StatusBadRequest)
			return
		}

		orgID, err := uuid.Parse(orgHeader)
		if err != nil || orgID == uuid.Nil {
			http.Error(w, `{"error":"X-Organization-ID must be a valid non-nil UUID"}`, http.StatusBadRequest)
			return
		}

		ctx := config.ContextWithOrg(r.Context(), orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// newStatusDB returns a StatusDB backed by the pool, or a nop for testing.
func (s *Server) newStatusDB() StatusDB {
	if s.pool != nil {
		return &poolStatusDB{pool: s.pool}
	}
	return &nopStatusDB{}
}

// newRouteHandler creates a RouteHandler wired to the appropriate stores.
func (s *Server) newRouteHandler() *RouteHandler {
	if s.configStore != nil && s.store != nil {
		return NewRouteHandler(s.configStore, s.store)
	}
	return NewRouteHandler(&nopRouteConfigStore{}, &nopRouteStore{})
}

// newBackendHandler creates a BackendHandler wired to the appropriate stores.
func (s *Server) newBackendHandler() *BackendHandler {
	if s.configStore != nil && s.store != nil {
		return NewBackendHandler(s.configStore, s.store)
	}
	return NewBackendHandler(&nopBackendConfigStore{}, &nopBackendStore{})
}

// newConsumerHandler creates a ConsumerHandler wired to the appropriate stores.
func (s *Server) newConsumerHandler() *ConsumerHandler {
	if s.configStore != nil && s.store != nil {
		return NewConsumerHandler(s.configStore, s.store)
	}
	return NewConsumerHandler(&nopConsumerConfigStore{}, &nopConsumerStore{})
}

// newPluginStore creates a PluginStoreInterface wired to the pool, or a nop for testing.
func (s *Server) newPluginStore() PluginStoreInterface {
	if s.pool != nil {
		return plugin.NewPostgresStore(s.pool)
	}
	return &nopPluginStore{}
}

// newAIHandler creates an AIHandler wired to the store.
func (s *Server) newAIHandler() *AIHandler {
	if s.store != nil {
		return NewAIHandler(s.store)
	}
	return NewAIHandler(&nopAIStore{})
}

// newConfigVersionHandler creates a ConfigVersionHandler wired to the store.
func (s *Server) newConfigVersionHandler() *ConfigVersionHandler {
	if s.store != nil {
		return NewConfigVersionHandler(s.store)
	}
	return NewConfigVersionHandler(&nopConfigVersionStore{})
}

// newEnvironmentHandler creates an EnvironmentHandler wired to the store.
func (s *Server) newEnvironmentHandler() *EnvironmentHandler {
	if s.store != nil {
		return NewEnvironmentHandler(s.store)
	}
	return NewEnvironmentHandler(&nopEnvironmentStore{})
}

// newAPIVersionHandler creates an APIVersionHandler wired to the store.
func (s *Server) newAPIVersionHandler() *APIVersionHandler {
	if s.store != nil {
		return NewAPIVersionHandler(s.store)
	}
	return NewAPIVersionHandler(&nopAPIVersionStore{})
}
