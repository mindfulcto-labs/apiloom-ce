package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// APIVersionHandler handles HTTP requests for API version CRUD + canary config.
type APIVersionHandler struct {
	versionStore APIVersionStore
}

// NewAPIVersionHandler creates an APIVersionHandler with the given store.
func NewAPIVersionHandler(versionStore APIVersionStore) *APIVersionHandler {
	return &APIVersionHandler{versionStore: versionStore}
}

// Routes mounts version + canary endpoints onto the chi router.
// Called as a sub-router under /routes/{id}/versions.
func (h *APIVersionHandler) Routes(r chi.Router) {
	r.Get("/", h.handleListVersions)
	r.Post("/", h.handleCreateVersion)
	r.Get("/{vid}/diff", h.handleDiffVersions)
}

// CanaryRoutes mounts canary endpoints onto the chi router.
// Called as a sub-router under /routes/{id}/canary.
func (h *APIVersionHandler) CanaryRoutes(r chi.Router) {
	r.Put("/", h.handleSetCanary)
	r.Delete("/", h.handleClearCanary)
	r.Get("/", h.handleGetCanary)
}

// handleListVersions returns all versions for a route.
func (h *APIVersionHandler) handleListVersions(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	versions, err := h.versionStore.ListAPIVersions(r.Context(), routeID)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, versions)
}

// createVersionRequest is the DTO for creating an API version.
type createVersionRequest struct {
	VersionTag string          `json:"version_tag"`
	Config     json.RawMessage `json:"config"`
	Changelog  string          `json:"changelog"`
	CreatedBy  string          `json:"created_by"`
}

// handleCreateVersion snapshots the current route config as a new named version.
func (h *APIVersionHandler) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req createVersionRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	cfgSnapshot := req.Config
	if cfgSnapshot == nil {
		cfgSnapshot = json.RawMessage(`{}`)
	}

	version := &domain.APIVersion{
		RouteID:    routeID,
		VersionTag: req.VersionTag,
		Config:     cfgSnapshot,
		Changelog:  req.Changelog,
		CreatedBy:  req.CreatedBy,
	}

	if err := version.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.versionStore.CreateAPIVersion(r.Context(), version); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, version)
}

// diffResponse is the DTO for a version diff.
type diffResponse struct {
	Diff           domain.ConfigDiff `json:"diff"`
	MigrationNotes []string          `json:"migration_notes"`
	IsBreaking     bool              `json:"is_breaking"`
}

// handleDiffVersions diffs two versions and returns structured diff + migration notes.
func (h *APIVersionHandler) handleDiffVersions(w http.ResponseWriter, r *http.Request) {
	vid, err := parseUUIDParam(r, "vid")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	compareToStr := r.URL.Query().Get("compare_to")
	if compareToStr == "" {
		WriteProblem(w, http.StatusBadRequest, "validation", "compare_to query parameter is required")
		return
	}
	compareTo, err := uuid.Parse(compareToStr)
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "compare_to must be a valid UUID")
		return
	}

	v1, err := h.versionStore.GetAPIVersion(r.Context(), vid)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	v2, err := h.versionStore.GetAPIVersion(r.Context(), compareTo)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	diff, err := domain.DiffConfigs(v1.Config, v2.Config)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "diff-error", "failed to compute config diff")
		return
	}

	notes := domain.GenerateMigrationNotes(diff)
	if notes == nil {
		notes = []string{}
	}

	resp := diffResponse{
		Diff:           diff,
		MigrationNotes: notes,
		IsBreaking:     domain.IsBreakingChange(diff),
	}

	WriteJSON(w, http.StatusOK, resp)
}

// setCanaryRequest is the DTO for setting canary config.
type setCanaryRequest struct {
	CanaryVersionID string `json:"canary_version_id"`
	Weight          int    `json:"weight"`
}

// handleSetCanary sets or updates the canary config for a route.
func (h *APIVersionHandler) handleSetCanary(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req setCanaryRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	canaryVersionID, err := uuid.Parse(req.CanaryVersionID)
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "canary_version_id must be a valid UUID")
		return
	}

	// Get existing versions to determine primary.
	versions, err := h.versionStore.ListAPIVersions(r.Context(), routeID)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}
	if len(versions) == 0 {
		WriteProblem(w, http.StatusBadRequest, "validation", "no versions exist for this route")
		return
	}

	// Primary is the most recent version that is not the canary.
	var primaryID uuid.UUID
	for _, v := range versions {
		if v.ID != canaryVersionID {
			primaryID = v.ID
			break
		}
	}
	if primaryID == uuid.Nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "cannot find primary version different from canary")
		return
	}

	canary := &domain.CanaryConfig{
		RouteID:          routeID,
		PrimaryVersionID: primaryID,
		CanaryVersionID:  canaryVersionID,
		CanaryWeight:     req.Weight,
	}
	if err := canary.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.versionStore.SetCanary(r.Context(), canary); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, canary)
}

// handleClearCanary removes the canary config for a route.
func (h *APIVersionHandler) handleClearCanary(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.versionStore.ClearCanary(r.Context(), routeID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetCanary returns the current canary config for a route.
func (h *APIVersionHandler) handleGetCanary(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	canary, err := h.versionStore.GetCanary(r.Context(), routeID)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	if canary == nil {
		WriteJSON(w, http.StatusOK, nil)
		return
	}

	WriteJSON(w, http.StatusOK, canary)
}
