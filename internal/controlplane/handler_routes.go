package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

const maxBodyBytes = 10 << 20 // 10MB

// RouteConfigStore abstracts the config.ConfigStore methods used by RouteHandler.
type RouteConfigStore interface {
	SaveRoute(ctx context.Context, rc *domain.RouteConfig) (*domain.RouteConfig, error)
	LoadRoute(ctx context.Context, id uuid.UUID) (*domain.RouteConfig, error)
	DeleteRoute(ctx context.Context, id uuid.UUID) error
}

// RouteStore abstracts the control plane Store methods used by RouteHandler.
type RouteStore interface {
	UpdateRoute(ctx context.Context, id uuid.UUID, req UpdateRouteRequest) (*domain.RouteConfig, error)
	ListRoutesPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.RouteConfig], error)
}

// RouteHandler handles HTTP requests for route CRUD operations.
type RouteHandler struct {
	configStore RouteConfigStore
	store       RouteStore
}

// NewRouteHandler creates a RouteHandler with the given stores.
func NewRouteHandler(configStore RouteConfigStore, store RouteStore) *RouteHandler {
	return &RouteHandler{
		configStore: configStore,
		store:       store,
	}
}

// Routes mounts route CRUD endpoints onto the given chi router.
func (h *RouteHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.getByID)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.remove)
}

func (h *RouteHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRouteRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	rc := &domain.RouteConfig{
		Name:     req.Name,
		Config:   req.Config,
		Metadata: req.Metadata,
	}

	saved, err := h.configStore.SaveRoute(r.Context(), rc)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, RouteResponseFromDomain(*saved))
}

func (h *RouteHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	route, err := h.configStore.LoadRoute(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, RouteResponseFromDomain(*route))
}

func (h *RouteHandler) list(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListRoutesPaged(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	resp := PagedResponse[RouteResponse]{
		Data:       make([]RouteResponse, 0, len(result.Data)),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
	for _, rc := range result.Data {
		resp.Data = append(resp.Data, RouteResponseFromDomain(rc))
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (h *RouteHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req UpdateRouteRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	updated, err := h.store.UpdateRoute(r.Context(), id, req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, RouteResponseFromDomain(*updated))
}

func (h *RouteHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.configStore.DeleteRoute(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Shared helpers ---

// parseUUIDParam extracts and parses a UUID from a chi URL parameter.
func parseUUIDParam(r *http.Request, param string) (uuid.UUID, error) {
	raw := chi.URLParam(r, param)
	return uuid.Parse(raw)
}

// decodeBody reads the request body (limited to maxBodyBytes) and JSON-decodes it.
func decodeBody(r *http.Request, v any) error {
	body := io.LimitReader(r.Body, maxBodyBytes+1)
	return json.NewDecoder(body).Decode(v)
}

// extractErrType extracts the short error type from a URN-formatted type string.
func extractErrType(urnType string) string {
	// "urn:flexgateway:error:not-found" -> "not-found"
	const prefix = "urn:flexgateway:error:"
	if len(urnType) > len(prefix) {
		return urnType[len(prefix):]
	}
	return urnType
}
