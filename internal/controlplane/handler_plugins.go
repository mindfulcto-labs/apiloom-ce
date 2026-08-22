package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mindfulcto-labs/apiloom-ce/internal/plugin"
)

// PluginStoreInterface abstracts the plugin store methods used by PluginHandler.
// Defined in consumer (handler) package per interfaces-in-consumer rule.
type PluginStoreInterface interface {
	Create(ctx context.Context, rec *plugin.PluginRecord) (*plugin.PluginRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*plugin.PluginRecord, error)
	List(ctx context.Context, params plugin.ListPluginParams) (*plugin.PagedPlugins, error)
	UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) (*plugin.PluginRecord, error)
	UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*plugin.PluginRecord, error)
	UpdateOrdering(ctx context.Context, id uuid.UUID, ordering int) (*plugin.PluginRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// PluginHandler provides HTTP handlers for plugin CRUD.
type PluginHandler struct {
	store PluginStoreInterface
}

// NewPluginHandler creates a new PluginHandler.
func NewPluginHandler(store PluginStoreInterface) *PluginHandler {
	return &PluginHandler{store: store}
}

// RegisterPluginRoutes mounts plugin endpoints on the given router.
func RegisterPluginRoutes(r chi.Router, store PluginStoreInterface) {
	h := NewPluginHandler(store)
	r.Route("/plugins", h.Routes)
}

// Routes mounts plugin CRUD endpoints.
func (h *PluginHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.getByID)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.remove)
	r.Patch("/{id}/enable", h.enable)
	r.Patch("/{id}/disable", h.disable)
}

// --- Request DTOs ---

// CreatePluginRequest is the DTO for creating a plugin.
type CreatePluginRequest struct {
	Name        string          `json:"name"`
	PluginType  string          `json:"plugin_type"`
	Config      json.RawMessage `json:"config,omitempty"`
	Target      string          `json:"target,omitempty"`
	GRPCAddress *string         `json:"grpc_address,omitempty"`
	Ordering    int             `json:"ordering,omitempty"`
}

// Validate checks required fields and constraints.
func (r CreatePluginRequest) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.PluginType != "native" && r.PluginType != "grpc" {
		return fmt.Errorf("plugin_type must be 'native' or 'grpc'")
	}
	if r.PluginType == "grpc" && (r.GRPCAddress == nil || *r.GRPCAddress == "") {
		return fmt.Errorf("grpc_address is required when plugin_type is 'grpc'")
	}
	if r.Target != "" && r.Target != "global" && r.Target != "route" && r.Target != "consumer" {
		return fmt.Errorf("target must be one of: global, route, consumer")
	}
	return nil
}

// UpdatePluginRequest is the DTO for updating a plugin.
type UpdatePluginRequest struct {
	Config   json.RawMessage `json:"config,omitempty"`
	Ordering *int            `json:"ordering,omitempty"`
}

// --- Handlers ---

func (h *PluginHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreatePluginRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation-error", err.Error())
		return
	}

	target := req.Target
	if target == "" {
		target = "global"
	}
	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	rec := &plugin.PluginRecord{
		Name:        req.Name,
		PluginType:  req.PluginType,
		Config:      cfg,
		Target:      target,
		GRPCAddress: req.GRPCAddress,
		Ordering:    req.Ordering,
	}

	result, err := h.store.Create(r.Context(), rec)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, result)
}

func (h *PluginHandler) list(w http.ResponseWriter, r *http.Request) {
	params := plugin.ListPluginParams{
		Cursor: r.URL.Query().Get("cursor"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			params.Limit = n
		}
	}
	if v := r.URL.Query().Get("enabled"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			params.IsEnabled = &b
		}
	}
	if v := r.URL.Query().Get("type"); v != "" {
		params.PluginType = v
	}

	result, err := h.store.List(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

func (h *PluginHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	p, err := h.store.Get(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, p)
}

func (h *PluginHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	var req UpdatePluginRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}

	var result *plugin.PluginRecord

	if req.Config != nil {
		result, err = h.store.UpdateConfig(r.Context(), id, req.Config)
		if err != nil {
			pe := ProblemFromError(err)
			WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
			return
		}
	}

	if req.Ordering != nil {
		result, err = h.store.UpdateOrdering(r.Context(), id, *req.Ordering)
		if err != nil {
			pe := ProblemFromError(err)
			WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
			return
		}
	}

	if result == nil {
		// No fields to update, just return current.
		result, err = h.store.Get(r.Context(), id)
		if err != nil {
			pe := ProblemFromError(err)
			WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
			return
		}
	}

	WriteJSON(w, http.StatusOK, result)
}

func (h *PluginHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PluginHandler) enable(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	result, err := h.store.UpdateEnabled(r.Context(), id, true)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

func (h *PluginHandler) disable(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	result, err := h.store.UpdateEnabled(r.Context(), id, false)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}
