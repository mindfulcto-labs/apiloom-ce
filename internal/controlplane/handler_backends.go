package controlplane

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// BackendConfigStore abstracts the config.ConfigStore methods used by BackendHandler.
type BackendConfigStore interface {
	SaveBackend(ctx context.Context, bc *domain.BackendConfig) (*domain.BackendConfig, error)
	LoadBackend(ctx context.Context, id uuid.UUID) (*domain.BackendConfig, error)
	DeleteBackend(ctx context.Context, id uuid.UUID) error
}

// BackendStore abstracts the control plane Store methods used by BackendHandler.
type BackendStore interface {
	UpdateBackend(ctx context.Context, id uuid.UUID, req UpdateBackendRequest) (*domain.BackendConfig, error)
	ListBackendsPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.BackendConfig], error)
}

// BackendHandler handles HTTP requests for backend CRUD operations.
type BackendHandler struct {
	configStore BackendConfigStore
	store       BackendStore
}

// NewBackendHandler creates a BackendHandler with the given stores.
func NewBackendHandler(configStore BackendConfigStore, store BackendStore) *BackendHandler {
	return &BackendHandler{
		configStore: configStore,
		store:       store,
	}
}

// Routes mounts backend CRUD endpoints onto the given chi router.
func (h *BackendHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.getByID)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.remove)
}

func (h *BackendHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateBackendRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	bc := &domain.BackendConfig{
		Name:     req.Name,
		Config:   req.Config,
		Metadata: req.Metadata,
	}

	saved, err := h.configStore.SaveBackend(r.Context(), bc)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, BackendResponseFromDomain(*saved))
}

func (h *BackendHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	backend, err := h.configStore.LoadBackend(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, BackendResponseFromDomain(*backend))
}

func (h *BackendHandler) list(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListBackendsPaged(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	resp := PagedResponse[BackendResponse]{
		Data:       make([]BackendResponse, 0, len(result.Data)),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
	for _, bc := range result.Data {
		resp.Data = append(resp.Data, BackendResponseFromDomain(bc))
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (h *BackendHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req UpdateBackendRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	updated, err := h.store.UpdateBackend(r.Context(), id, req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, BackendResponseFromDomain(*updated))
}

func (h *BackendHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.configStore.DeleteBackend(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
