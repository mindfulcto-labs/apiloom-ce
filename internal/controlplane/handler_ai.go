package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxAIBodyBytes = 1 << 20 // 1 MB

// AIStoreInterface abstracts the store methods used by AIHandler.
// Defined in consumer (handler) package per interfaces-in-consumer rule.
type AIStoreInterface interface {
	CreateAIModel(ctx context.Context, req CreateAIModelRequest) (*AIModelRow, error)
	ListAIModelsPaged(ctx context.Context, params ListParams) (*PagedResponse[AIModelRow], error)
	GetAIModel(ctx context.Context, id uuid.UUID) (*AIModelRow, error)
	UpdateAIModel(ctx context.Context, id uuid.UUID, req UpdateAIModelRequest) (*AIModelRow, error)
	DeleteAIModel(ctx context.Context, id uuid.UUID) error

	CreateTokenBudget(ctx context.Context, req CreateTokenBudgetRequest) (*TokenBudgetRow, error)
	ListTokenBudgetsPaged(ctx context.Context, params ListParams) (*PagedResponse[TokenBudgetRow], error)
	GetTokenBudget(ctx context.Context, id uuid.UUID) (*TokenBudgetRow, error)
	UpdateTokenBudget(ctx context.Context, id uuid.UUID, req UpdateTokenBudgetRequest) (*TokenBudgetRow, error)
	DeleteTokenBudget(ctx context.Context, id uuid.UUID) error
}

// AIHandler provides HTTP handlers for AI model and token budget CRUD.
type AIHandler struct {
	store AIStoreInterface
}

// NewAIHandler creates a new AIHandler.
func NewAIHandler(store AIStoreInterface) *AIHandler {
	return &AIHandler{store: store}
}

// Routes mounts AI model and token budget endpoints on the given router.
func (h *AIHandler) Routes(r chi.Router) {
	r.Route("/ai/models", func(r chi.Router) {
		r.Post("/", h.createModel)
		r.Get("/", h.listModels)
		r.Get("/{id}", h.getModel)
		r.Put("/{id}", h.updateModel)
		r.Delete("/{id}", h.deleteModel)
	})
	r.Route("/ai/budgets", func(r chi.Router) {
		r.Post("/", h.createBudget)
		r.Get("/", h.listBudgets)
		r.Get("/{id}", h.getBudget)
		r.Put("/{id}", h.updateBudget)
		r.Delete("/{id}", h.deleteBudget)
	})
}

// --- AI Model handlers ---

func (h *AIHandler) createModel(w http.ResponseWriter, r *http.Request) {
	var req CreateAIModelRequest
	if err := decodeAIBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation-error", err.Error())
		return
	}

	model, err := h.store.CreateAIModel(r.Context(), req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, model)
}

func (h *AIHandler) listModels(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListAIModelsPaged(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

func (h *AIHandler) getModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	model, err := h.store.GetAIModel(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, model)
}

func (h *AIHandler) updateModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	var req UpdateAIModelRequest
	if err := decodeAIBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation-error", err.Error())
		return
	}

	model, err := h.store.UpdateAIModel(r.Context(), id, req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, model)
}

func (h *AIHandler) deleteModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	if err := h.store.DeleteAIModel(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Token Budget handlers ---

func (h *AIHandler) createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenBudgetRequest
	if err := decodeAIBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation-error", err.Error())
		return
	}

	budget, err := h.store.CreateTokenBudget(r.Context(), req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, budget)
}

func (h *AIHandler) listBudgets(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListTokenBudgetsPaged(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

func (h *AIHandler) getBudget(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	budget, err := h.store.GetTokenBudget(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, budget)
}

func (h *AIHandler) updateBudget(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	var req UpdateTokenBudgetRequest
	if err := decodeAIBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-request", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation-error", err.Error())
		return
	}

	budget, err := h.store.UpdateTokenBudget(r.Context(), id, req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, budget)
}

func (h *AIHandler) deleteBudget(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "invalid-id", err.Error())
		return
	}

	if err := h.store.DeleteTokenBudget(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, pe.Type, pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func decodeAIBody(r *http.Request, v any) error {
	body := io.LimitReader(r.Body, maxAIBodyBytes+1)
	return json.NewDecoder(body).Decode(v)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
