package controlplane

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mindfulcto-labs/apiloom-ce/internal/config"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// EnvironmentHandler handles HTTP requests for environment CRUD + promotion.
type EnvironmentHandler struct {
	store EnvironmentStore
}

// NewEnvironmentHandler creates an EnvironmentHandler with the given store.
func NewEnvironmentHandler(store EnvironmentStore) *EnvironmentHandler {
	return &EnvironmentHandler{store: store}
}

// Routes mounts environment endpoints onto the given chi router.
func (h *EnvironmentHandler) Routes(r chi.Router) {
	r.Get("/", h.HandleListEnvironments)
	r.Post("/", h.HandleCreateEnvironment)
	r.Delete("/{id}", h.handleDelete)
	r.Post("/{id}/promote", h.HandlePromoteEnvironment)
}

// HandleListEnvironments returns all environments for the org.
func (h *EnvironmentHandler) HandleListEnvironments(w http.ResponseWriter, r *http.Request) {
	orgID, err := config.OrgFromContext(r.Context())
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "organization context required")
		return
	}

	envs, err := h.store.ListEnvironments(r.Context(), orgID)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, envs)
}

// createEnvironmentRequest is the DTO for creating an environment.
type createEnvironmentRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsProduction bool   `json:"is_production"`
	SortOrder    int    `json:"sort_order"`
}

// HandleCreateEnvironment creates a new environment.
func (h *EnvironmentHandler) HandleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	orgID, err := config.OrgFromContext(r.Context())
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "organization context required")
		return
	}

	var req createEnvironmentRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	env := &domain.Environment{
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		IsProduction:   req.IsProduction,
		SortOrder:      req.SortOrder,
	}
	if err := env.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.store.CreateEnvironment(r.Context(), env); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, env)
}

// handleDelete removes an environment (production envs are blocked).
func (h *EnvironmentHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	orgID, err := config.OrgFromContext(r.Context())
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "organization context required")
		return
	}

	envID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.store.DeleteEnvironment(r.Context(), orgID, envID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// promoteRequest is the DTO for promoting between environments.
type promoteRequest struct {
	TargetEnvironmentID string `json:"target_environment_id"`
}

// HandlePromoteEnvironment promotes config from source env to target env.
func (h *EnvironmentHandler) HandlePromoteEnvironment(w http.ResponseWriter, r *http.Request) {
	orgID, err := config.OrgFromContext(r.Context())
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "organization context required")
		return
	}

	sourceEnvID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req promoteRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	targetEnvID, err := parseUUIDString(req.TargetEnvironmentID)
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "target_environment_id must be a valid UUID")
		return
	}

	result, err := h.store.PromoteEnvironment(r.Context(), orgID, sourceEnvID, targetEnvID)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// parseUUIDString parses a UUID string and returns an error if invalid.
func parseUUIDString(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
