package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// ConsumerConfigStore abstracts config.ConfigStore methods for consumer and API key operations.
type ConsumerConfigStore interface {
	CreateConsumer(ctx context.Context, name string, metadata json.RawMessage) (*domain.Consumer, error)
	GetConsumer(ctx context.Context, id uuid.UUID) (*domain.Consumer, error)
	DeleteConsumer(ctx context.Context, id uuid.UUID) error
	CreateAPIKey(ctx context.Context, consumerID uuid.UUID, permissions json.RawMessage) (string, *domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID) error
	RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, gracePeriod time.Duration) (string, *domain.APIKey, error)
	GetAPIKey(ctx context.Context, id uuid.UUID) (*domain.APIKey, error)
}

// ConsumerStore abstracts control plane Store methods for consumer operations.
type ConsumerStore interface {
	UpdateConsumer(ctx context.Context, id uuid.UUID, req UpdateConsumerRequest) (*domain.Consumer, error)
	ListConsumersPaged(ctx context.Context, params ListParams) (*PagedResponse[domain.Consumer], error)
	ListAPIKeys(ctx context.Context, consumerID uuid.UUID, params ListParams) (*PagedResponse[domain.APIKey], error)
}

// ConsumerHandler handles HTTP requests for consumer and API key CRUD operations.
type ConsumerHandler struct {
	configStore ConsumerConfigStore
	store       ConsumerStore
}

// NewConsumerHandler creates a ConsumerHandler with the given stores.
func NewConsumerHandler(configStore ConsumerConfigStore, store ConsumerStore) *ConsumerHandler {
	return &ConsumerHandler{
		configStore: configStore,
		store:       store,
	}
}

// Routes mounts consumer and API key CRUD endpoints onto the given chi router.
func (h *ConsumerHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.getByID)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.remove)

	// API key sub-resource endpoints
	r.Post("/{id}/keys", h.createKey)
	r.Get("/{id}/keys", h.listKeys)
	r.Post("/{id}/keys/{keyId}/rotate", h.rotateKey)
	r.Delete("/{id}/keys/{keyId}", h.revokeKey)
}

func (h *ConsumerHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateConsumerRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	consumer, err := h.configStore.CreateConsumer(r.Context(), req.Name, req.Metadata)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, ConsumerResponseFromDomain(*consumer))
}

func (h *ConsumerHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	consumer, err := h.configStore.GetConsumer(r.Context(), id)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, ConsumerResponseFromDomain(*consumer))
}

func (h *ConsumerHandler) list(w http.ResponseWriter, r *http.Request) {
	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListConsumersPaged(r.Context(), params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	resp := PagedResponse[ConsumerResponse]{
		Data:       make([]ConsumerResponse, 0, len(result.Data)),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
	for _, c := range result.Data {
		resp.Data = append(resp.Data, ConsumerResponseFromDomain(c))
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (h *ConsumerHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var req UpdateConsumerRequest
	if err := decodeBody(r, &req); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	updated, err := h.store.UpdateConsumer(r.Context(), id, req)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusOK, ConsumerResponseFromDomain(*updated))
}

func (h *ConsumerHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	if err := h.configStore.DeleteConsumer(r.Context(), id); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- API Key sub-resource handlers ---

// APIKeyCreateResponse wraps the raw key (shown once) and the key metadata.
type APIKeyCreateResponse struct {
	Key    string         `json:"key"`
	APIKey APIKeyResponse `json:"api_key"`
}

func (h *ConsumerHandler) createKey(w http.ResponseWriter, r *http.Request) {
	consumerID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	// Verify consumer exists
	if _, err := h.configStore.GetConsumer(r.Context(), consumerID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	// Parse optional permissions from request body.
	var permissions json.RawMessage
	if r.ContentLength > 0 {
		var req CreateAPIKeyRequest
		if err := decodeBody(r, &req); err != nil {
			WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		if err := req.Validate(); err != nil {
			WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		if len(req.Permissions) > 0 {
			permissions = req.Permissions
		}
	}

	rawKey, apiKey, err := h.configStore.CreateAPIKey(r.Context(), consumerID, permissions)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, APIKeyCreateResponse{
		Key:    rawKey,
		APIKey: APIKeyResponseFromDomain(*apiKey),
	})
}

func (h *ConsumerHandler) listKeys(w http.ResponseWriter, r *http.Request) {
	consumerID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	// Verify consumer exists
	if _, err := h.configStore.GetConsumer(r.Context(), consumerID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	params := ParseListParams(r.URL.Query())

	result, err := h.store.ListAPIKeys(r.Context(), consumerID, params)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	resp := PagedResponse[APIKeyResponse]{
		Data:       make([]APIKeyResponse, 0, len(result.Data)),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
	for _, k := range result.Data {
		resp.Data = append(resp.Data, APIKeyResponseFromDomain(k))
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (h *ConsumerHandler) rotateKey(w http.ResponseWriter, r *http.Request) {
	consumerID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	keyID, err := parseUUIDParam(r, "keyId")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	// BOLA check: verify key belongs to this consumer
	if err := h.verifyKeyOwnership(r.Context(), consumerID, keyID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	rawKey, newKey, err := h.configStore.RotateAPIKey(r.Context(), keyID, 1*time.Hour)
	if err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	WriteJSON(w, http.StatusCreated, APIKeyCreateResponse{
		Key:    rawKey,
		APIKey: APIKeyResponseFromDomain(*newKey),
	})
}

func (h *ConsumerHandler) revokeKey(w http.ResponseWriter, r *http.Request) {
	consumerID, err := parseUUIDParam(r, "id")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	keyID, err := parseUUIDParam(r, "keyId")
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	// BOLA check: verify key belongs to this consumer
	if err := h.verifyKeyOwnership(r.Context(), consumerID, keyID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	if err := h.configStore.RevokeAPIKey(r.Context(), keyID); err != nil {
		pe := ProblemFromError(err)
		WriteProblem(w, pe.Status, extractErrType(pe.Type), pe.Detail)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// verifyKeyOwnership checks that an API key belongs to the specified consumer.
// Returns domain.ErrNotFound if the key doesn't exist or belongs to a different consumer
// (returns 404 instead of 403 to avoid leaking resource existence per BOLA prevention rules).
func (h *ConsumerHandler) verifyKeyOwnership(ctx context.Context, consumerID, keyID uuid.UUID) error {
	key, err := h.configStore.GetAPIKey(ctx, keyID)
	if err != nil {
		return err
	}

	if key.ConsumerID != consumerID {
		return domain.ErrNotFound
	}

	return nil
}
