package aiproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
	"github.com/cto-venkat/apiloom-ce/internal/ai/budget"
	"github.com/cto-venkat/apiloom-ce/internal/ai/provider"
	"github.com/cto-venkat/apiloom-ce/internal/ai/router"
	"github.com/cto-venkat/apiloom-ce/internal/ai/token"
	"github.com/cto-venkat/apiloom-ce/internal/block/auth"
	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// mockRouter implements router.Router for testing.
type mockRouter struct {
	routeFn func(req *ai.ChatRequest, models []ai.Model) (ai.Model, error)
}

func (m *mockRouter) Route(req *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	if m.routeFn != nil {
		return m.routeFn(req, models)
	}
	if len(models) == 0 {
		return ai.Model{}, router.ErrNoModelsAvailable
	}
	return models[0], nil
}

func validChatRequestBody(t *testing.T) []byte {
	t.Helper()
	req := ai.ChatRequest{
		Model:    "gpt-4",
		Messages: []ai.ChatMessage{{Role: "user", Content: "Hello"}},
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	return b
}

func validChatResponse() string {
	resp := ai.ChatResponse{
		ID:      "chatcmpl-1",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []ai.Choice{
			{
				Index:        0,
				Message:      ai.ChatMessage{Role: "assistant", Content: "Hi there"},
				FinishReason: "stop",
			},
		},
		Usage: ai.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func setupHandler(t *testing.T, opts ...func(*handlerSetup)) *Handler {
	t.Helper()
	s := &handlerSetup{
		providers: map[string]provider.Provider{
			"openai": newSuccessProvider(validChatResponse(), 200),
		},
		router: &mockRouter{},
		budget: budget.NewMemoryBudgetStore(1000000, 100.0, budget.HardEnforcement, nil),
		config: AIProxyConfig{
			Models: []ai.Model{
				{Provider: "openai", ModelName: "gpt-4", IsActive: true, InputPricePerToken: 0.00003, OutputPricePerToken: 0.00006},
			},
			DefaultStrategy:     router.StrategyRoundRobin,
			FallbackEnabled:     false,
			BudgetEnabled:       true,
			MaxRequestBodyBytes: DefaultMaxRequestBodyBytes,
		},
	}
	for _, opt := range opts {
		opt(s)
	}

	logger := newDiscardLogger()
	return NewHandler(s.config, s.providers, s.router, s.budget, token.NewCounter(), logger)
}

type handlerSetup struct {
	providers map[string]provider.Provider
	router    router.Router
	budget    budget.BudgetStore
	config    AIProxyConfig
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func withConsumerContext(r *http.Request) *http.Request {
	consumerID := uuid.New()
	consumer := &domain.Consumer{ID: consumerID}
	ctx := auth.SetConsumer(r.Context(), consumer)
	return r.WithContext(ctx)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	t.Run("non-streaming success", func(t *testing.T) {
		t.Parallel()
		h := setupHandler(t)

		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		assert.NotEmpty(t, rec.Header().Get("X-FlexGateway-Model-Used"))
		assert.NotEmpty(t, rec.Header().Get("X-FlexGateway-Tokens-Used"))

		var resp ai.ChatResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "chatcmpl-1", resp.ID)
	})

	t.Run("streaming success", func(t *testing.T) {
		t.Parallel()

		sseBody := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n"

		h := setupHandler(t, func(s *handlerSetup) {
			s.providers["openai"] = &mockProvider{
				callFn: func(_ context.Context, _ any, streaming bool) (*ai.ProviderResult, error) {
					if streaming {
						return &ai.ProviderResult{
							Body:       io.NopCloser(strings.NewReader(sseBody)),
							StatusCode: 200,
							Streaming:  true,
						}, nil
					}
					return &ai.ProviderResult{
						Body:       io.NopCloser(strings.NewReader(validChatResponse())),
						StatusCode: 200,
					}, nil
				},
			}
		})

		chatReq := ai.ChatRequest{
			Model:    "gpt-4",
			Messages: []ai.ChatMessage{{Role: "user", Content: "Hello"}},
			Stream:   true,
		}
		body, _ := json.Marshal(chatReq)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		w := newTestResponseWriter()
		h.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.statusCode)
		assert.Equal(t, "text/event-stream", w.header.Get("Content-Type"))
		assert.Contains(t, w.body.String(), "data: [DONE]")
	})

	t.Run("fallback: first fails, second succeeds", func(t *testing.T) {
		t.Parallel()

		h := setupHandler(t, func(s *handlerSetup) {
			s.config.FallbackEnabled = true
			s.config.Models = []ai.Model{
				{Provider: "openai", ModelName: "gpt-4", IsActive: true, InputPricePerToken: 0.00003, OutputPricePerToken: 0.00006},
				{Provider: "anthropic", ModelName: "claude-3", IsActive: true, InputPricePerToken: 0.00003, OutputPricePerToken: 0.00006},
			}
			s.providers["openai"] = newFailProvider(500)
			s.providers["anthropic"] = newSuccessProvider(validChatResponse(), 200)
		})

		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "anthropic/claude-3", rec.Header().Get("X-FlexGateway-Model-Used"))
	})

	t.Run("all fallbacks fail returns 502", func(t *testing.T) {
		t.Parallel()

		h := setupHandler(t, func(s *handlerSetup) {
			s.config.FallbackEnabled = true
			s.config.Models = []ai.Model{
				{Provider: "openai", ModelName: "gpt-4", IsActive: true},
				{Provider: "anthropic", ModelName: "claude-3", IsActive: true},
			}
			s.providers["openai"] = newFailProvider(500)
			s.providers["anthropic"] = newFailProvider(503)
		})

		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("budget exceeded returns 429", func(t *testing.T) {
		t.Parallel()

		h := setupHandler(t, func(s *handlerSetup) {
			s.budget = budget.NewMemoryBudgetStore(1, 0.0001, budget.HardEnforcement, nil)
		})

		// First request to exhaust budget.
		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		// Second request should be rejected.
		body = validChatRequestBody(t)
		req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("cost ceiling exceeded returns 402", func(t *testing.T) {
		t.Parallel()

		ceiling := 0.0000001 // very low ceiling
		h := setupHandler(t, func(s *handlerSetup) {
			s.budget = budget.NewMemoryBudgetStore(1000000, 100.0, budget.HardEnforcement, &ceiling)
		})

		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusPaymentRequired, rec.Code)
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		t.Parallel()
		h := setupHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{bad json"))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("oversized body returns 413", func(t *testing.T) {
		t.Parallel()
		h := setupHandler(t, func(s *handlerSetup) {
			s.config.MaxRequestBodyBytes = 10 // 10 bytes max
		})

		body := validChatRequestBody(t)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withConsumerContext(req)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("no consumer skips budget check", func(t *testing.T) {
		t.Parallel()
		h := setupHandler(t)

		body := validChatRequestBody(t)
		// No consumer in context -- budget check should be skipped.
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
