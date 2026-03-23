package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// ConfigVersionListItem is the list response DTO (no snapshot for bandwidth).
type ConfigVersionListItem struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ConfigVersionHandler handles HTTP requests for config version operations.
type ConfigVersionHandler struct {
	store ConfigVersionStore
}

// NewConfigVersionHandler creates a ConfigVersionHandler with the given store.
func NewConfigVersionHandler(store ConfigVersionStore) *ConfigVersionHandler {
	return &ConfigVersionHandler{store: store}
}

// Routes mounts config version endpoints onto the given chi router.
func (h *ConfigVersionHandler) Routes(r chi.Router) {
	r.Get("/versions", h.list)
	r.Get("/versions/{version}", h.get)
	r.Post("/rollback", h.rollback)
}

func (h *ConfigVersionHandler) list(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())
	result, err := h.store.ListVersions(r.Context(), params)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "internal", "failed to list config versions")
		return
	}

	// Strip snapshot from list response.
	items := make([]ConfigVersionListItem, len(result.Data))
	for i, v := range result.Data {
		items[i] = ConfigVersionListItem{
			ID:          v.ID.String(),
			Version:     v.Version,
			Author:      v.Author,
			Description: v.Description,
			CreatedAt:   v.CreatedAt,
		}
	}

	resp := &PagedResponse[ConfigVersionListItem]{
		Data:       items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (h *ConfigVersionHandler) get(w http.ResponseWriter, r *http.Request) {
	versionStr := chi.URLParam(r, "version")
	version, err := strconv.Atoi(versionStr)
	if err != nil || version < 1 {
		WriteProblem(w, http.StatusBadRequest, "validation", "version must be a positive integer")
		return
	}

	cv, err := h.store.GetVersion(r.Context(), version)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			WriteProblem(w, http.StatusNotFound, "not-found", "config version not found")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "internal", "failed to get config version")
		return
	}

	WriteJSON(w, http.StatusOK, cv)
}

// rollbackRequest is the request body for the rollback endpoint.
type rollbackRequest struct {
	ToVersion int `json:"to_version"`
}

func (h *ConfigVersionHandler) rollback(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", "invalid request body")
		return
	}
	if req.ToVersion < 1 {
		WriteProblem(w, http.StatusBadRequest, "validation", "to_version must be a positive integer")
		return
	}

	cv, err := h.store.RollbackToVersion(r.Context(), req.ToVersion)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			WriteProblem(w, http.StatusNotFound, "not-found", "target config version not found")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "internal", "failed to rollback config version")
		return
	}

	WriteJSON(w, http.StatusOK, cv)
}
