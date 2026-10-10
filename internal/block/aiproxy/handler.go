package aiproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/budget"
	ailog "github.com/mindfulcto-labs/apiloom-ce/internal/ai/log"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/router"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/token"
	"github.com/mindfulcto-labs/apiloom-ce/internal/block/auth"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/observability"
)

// Handler is the AI proxy HTTP handler that processes /v1/chat/completions requests.
type Handler struct {
	providers   map[string]provider.Provider
	router      router.Router
	budget      budget.BudgetStore
	counter     *token.Counter
	fallback    *FallbackChain
	logger      *slog.Logger
	config      AIProxyConfig
	usageLogger ailog.UsageLogger
	aiLogger    *ailog.AILogger
	logLevel    ailog.LogLevel
	aiMetrics   *observability.AIMetrics
}

// HandlerOption configures optional Handler features.
type HandlerOption func(*Handler)

// WithUsageLogger sets the usage logger for recording AI usage to the database.
func WithUsageLogger(ul ailog.UsageLogger) HandlerOption {
	return func(h *Handler) {
		h.usageLogger = ul
	}
}

// WithAILogger sets the AI request/response logger with PII redaction.
func WithAILogger(al *ailog.AILogger, level ailog.LogLevel) HandlerOption {
	return func(h *Handler) {
		h.aiLogger = al
		h.logLevel = level
	}
}

// WithAIMetrics sets the AI-specific Prometheus metrics recorder.
// When nil, no AI metrics are recorded.
func WithAIMetrics(m *observability.AIMetrics) HandlerOption {
	return func(h *Handler) {
		h.aiMetrics = m
	}
}

// NewHandler creates a new AI proxy handler.
func NewHandler(
	config AIProxyConfig,
	providers map[string]provider.Provider,
	rt router.Router,
	bs budget.BudgetStore,
	counter *token.Counter,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	if config.MaxRequestBodyBytes == 0 {
		config.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}
	h := &Handler{
		providers: providers,
		router:    rt,
		budget:    bs,
		counter:   counter,
		logger:    logger,
		config:    config,
	}
	for _, opt := range opts {
		opt(h)
	}
	h.fallback = NewFallbackChain(providers, logger)
	h.fallback.aiMetrics = h.aiMetrics
	return h
}

// ServeHTTP handles POST /v1/chat/completions requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Parse request body with size limit per security.md.
	chatReq, err := h.parseRequest(r)
	if err != nil {
		h.writeProblem(w, err)
		return
	}

	ctx := r.Context()
	consumer := auth.GetConsumer(ctx)

	// 2. Route to model.
	selectedModel, err := h.router.Route(chatReq, h.config.Models)
	if err != nil {
		h.writeProblem(w, domain.NewProblemError(
			http.StatusBadGateway,
			"no_model_available",
			"no model available for request",
		))
		return
	}

	// 3. Token estimation.
	estimatedTokens, _ := h.counter.EstimateTokens(selectedModel.Provider, chatReq.Messages)
	estimatedCost := h.counter.EstimateCost(selectedModel, estimatedTokens, 0)

	// 4. Budget enforcement (only if consumer and budget enabled).
	var budgetKey *budget.BudgetKey
	if consumer != nil && h.config.BudgetEnabled {
		bk := budget.BudgetKey{ConsumerID: &consumer.ID}
		budgetKey = &bk

		// Check per-request cost ceiling.
		if err := h.budget.CheckCeiling(ctx, bk, estimatedCost); err != nil {
			if errors.Is(err, budget.ErrCeilingExceeded) {
				h.writeProblem(w, domain.NewProblemError(
					http.StatusPaymentRequired,
					"cost_ceiling_exceeded",
					"estimated cost exceeds per-request ceiling",
				))
				return
			}
			h.writeProblem(w, domain.NewProblemError(
				http.StatusInternalServerError,
				"budget_error",
				"budget check failed",
			))
			return
		}

		// Reserve budget.
		if err := h.budget.Reserve(ctx, bk, estimatedTokens, estimatedCost); err != nil {
			if errors.Is(err, budget.ErrBudgetExceeded) {
				h.writeProblem(w, domain.NewProblemError(
					http.StatusTooManyRequests,
					"budget_exceeded",
					"token budget exceeded",
				))
				return
			}
			h.writeProblem(w, domain.NewProblemError(
				http.StatusInternalServerError,
				"budget_error",
				"budget reservation failed",
			))
			return
		}
	}

	// 5. Defer budget refund with detached context.
	defer func() {
		if budgetKey == nil {
			return
		}
		refundCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Refund will be called with actual usage after reconciliation.
		// At this point actualTokens is set by the response path below.
		_ = h.budget.Refund(refundCtx, *budgetKey, estimatedTokens, 0, 0)
	}()

	// 6. Call provider (with fallback if enabled).
	var result *ai.ProviderResult
	var modelUsed ai.Model

	if h.config.FallbackEnabled {
		result, modelUsed, err = h.fallback.Execute(ctx, chatReq, h.config.Models)
	} else {
		result, modelUsed, err = h.callSingleProvider(ctx, chatReq, selectedModel)
	}

	if err != nil {
		h.writeProblem(w, err)
		return
	}
	defer func() { _ = result.Body.Close() }()

	modelHeader := modelUsed.Provider + "/" + modelUsed.ModelName

	startTime := time.Now()

	// 7. Handle streaming vs non-streaming response.
	if chatReq.Stream && result.Streaming {
		accum := &TokenAccumulator{}
		streamErr := StreamSSE(ctx, w, result.Body, modelUsed.Provider, accum)
		if streamErr != nil {
			h.logger.Error("streaming error", "error", streamErr)
		}
		// Reconcile with accumulated usage after stream completes.
		usage := accum.Total()
		h.reconcileAndRefund(budgetKey, estimatedTokens, usage)
		h.logRequest(ctx, consumer, modelUsed, usage, time.Since(startTime), true, chatReq, "")
		return
	}

	// Non-streaming: read and translate response.
	respBody, err := io.ReadAll(io.LimitReader(result.Body, h.config.MaxRequestBodyBytes))
	if err != nil {
		h.writeProblem(w, domain.NewProblemError(
			http.StatusBadGateway,
			"provider_read_error",
			"failed to read provider response",
		))
		return
	}

	p, ok := h.providers[modelUsed.Provider]
	if !ok {
		h.writeProblem(w, domain.NewProblemError(
			http.StatusInternalServerError,
			"provider_missing",
			"provider not found after call",
		))
		return
	}

	chatResp, err := p.TranslateResponse(respBody, modelUsed.ModelName)
	if err != nil {
		h.writeProblem(w, domain.NewProblemError(
			http.StatusBadGateway,
			"provider_translate_error",
			"failed to translate provider response",
		))
		return
	}

	// Reconcile tokens.
	h.reconcileAndRefund(budgetKey, estimatedTokens, chatResp.Usage)
	h.logRequest(ctx, consumer, modelUsed, chatResp.Usage, time.Since(startTime), false, chatReq, responseContent(chatResp))

	tokensUsed := chatResp.Usage.TotalTokens

	// Set headers and write response.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-FlexGateway-Model-Used", modelHeader)
	w.Header().Set("X-FlexGateway-Tokens-Used", strconv.FormatInt(tokensUsed, 10))

	respJSON, err := json.Marshal(chatResp)
	if err != nil {
		h.writeProblem(w, domain.NewProblemError(
			http.StatusInternalServerError,
			"marshal_error",
			"failed to marshal response",
		))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respJSON)
}

// parseRequest reads and validates the request body.
func (h *Handler) parseRequest(r *http.Request) (*ai.ChatRequest, error) {
	// Read with maxBytes+1 to detect oversized bodies per security.md pattern.
	limited := io.LimitReader(r.Body, h.config.MaxRequestBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, domain.NewProblemError(
			http.StatusBadRequest,
			"read_error",
			"failed to read request body",
		)
	}

	if int64(len(body)) > h.config.MaxRequestBodyBytes {
		return nil, domain.NewProblemError(
			http.StatusRequestEntityTooLarge,
			"body_too_large",
			fmt.Sprintf("request body exceeds %d bytes", h.config.MaxRequestBodyBytes),
		)
	}

	var req ai.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, domain.NewProblemError(
			http.StatusBadRequest,
			"invalid_json",
			"request body is not valid JSON",
		)
	}

	return &req, nil
}

// callSingleProvider calls a single provider without fallback.
func (h *Handler) callSingleProvider(ctx context.Context, req *ai.ChatRequest, model ai.Model) (*ai.ProviderResult, ai.Model, error) {
	p, ok := h.providers[model.Provider]
	if !ok {
		return nil, model, domain.NewProblemError(
			http.StatusBadGateway,
			"provider_not_found",
			fmt.Sprintf("no provider registered for %s", model.Provider),
		)
	}

	providerReq, err := p.Translate(req)
	if err != nil {
		return nil, model, domain.NewProblemError(
			http.StatusBadGateway,
			"translate_error",
			"failed to translate request for provider",
		)
	}

	result, err := p.Call(ctx, providerReq, req.Stream)
	if err != nil {
		if h.aiMetrics != nil {
			h.aiMetrics.RecordProviderError(model.Provider, "call_error")
		}
		return nil, model, domain.NewProblemError(
			http.StatusBadGateway,
			"provider_error",
			"upstream provider call failed",
		)
	}

	return result, model, nil
}

// reconcileAndRefund reconciles actual token usage and refunds budget delta.
func (h *Handler) reconcileAndRefund(budgetKey *budget.BudgetKey, estimated int64, actual ai.Usage) {
	if budgetKey == nil {
		return
	}

	actualCost := float64(0) // will be calculated by budget store or caller
	refundCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = h.budget.Refund(refundCtx, *budgetKey, estimated, actual.TotalTokens, actualCost)
}

// logRequest records usage and AI logs after a request completes.
// Both usageLogger and aiLogger are optional (nil-safe).
func (h *Handler) logRequest(
	ctx context.Context,
	consumer *domain.Consumer,
	model ai.Model,
	usage ai.Usage,
	latency time.Duration,
	streaming bool,
	chatReq *ai.ChatRequest,
	responseText string,
) {
	latencyMs := latency.Milliseconds()

	// Usage logging with detached context to survive client disconnects.
	if h.usageLogger != nil {
		logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		entry := ailog.UsageEntry{
			Provider:         model.Provider,
			Model:            model.ModelName,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			LatencyMs:        latencyMs,
			IsStreaming:       streaming,
		}
		if consumer != nil {
			entry.ConsumerID = &consumer.ID
		}

		if err := h.usageLogger.LogUsage(logCtx, entry); err != nil {
			h.logger.Error("failed to log usage", "error", err)
		}
	}

	// Record AI-specific Prometheus metrics (nil-safe).
	if h.aiMetrics != nil {
		var consumerLabel string
		if consumer != nil {
			consumerLabel = consumer.ID.String()
		}
		h.aiMetrics.ObserveRequest(
			model.Provider, model.ModelName, consumerLabel,
			usage.PromptTokens, usage.CompletionTokens,
			0, // Cost calculated upstream; pass 0 for now.
			latency,
		)
	}

	// AI request/response logging with PII redaction.
	if h.aiLogger != nil {
		var consumerID string
		if consumer != nil {
			consumerID = consumer.ID.String()
		}

		logEntry := ailog.LogEntry{
			Timestamp:        time.Now(),
			ConsumerID:       consumerID,
			Provider:         model.Provider,
			Model:            model.ModelName,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			LatencyMs:        latencyMs,
			IsStreaming:       streaming,
			Prompt:           promptContent(chatReq),
			Response:         responseText,
		}
		h.aiLogger.Log(ctx, h.logLevel, logEntry)
	}
}

// promptContent extracts a string representation of the prompt messages.
func promptContent(req *ai.ChatRequest) string {
	if req == nil || len(req.Messages) == 0 {
		return ""
	}
	var parts []string
	for _, m := range req.Messages {
		if s, ok := m.Content.(string); ok {
			parts = append(parts, m.Role+": "+s)
		}
	}
	return strings.Join(parts, "\n")
}

// responseContent extracts a string representation of the response choices.
func responseContent(resp *ai.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	if s, ok := resp.Choices[0].Message.Content.(string); ok {
		return s
	}
	return ""
}

// writeProblem writes a ProblemError or generic error as an HTTP response.
// Never forwards raw provider errors per security.md.
func (h *Handler) writeProblem(w http.ResponseWriter, err error) {
	var pe *domain.ProblemError
	if errors.As(err, &pe) {
		w.Header().Set("Content-Type", "application/problem+json")
		body, marshalErr := json.Marshal(pe)
		if marshalErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(pe.StatusCode())
		_, _ = w.Write(body)
		return
	}

	// For any non-ProblemError, return a generic 502. Never expose raw error details.
	pe = domain.NewProblemError(
		http.StatusBadGateway,
		"internal_error",
		"an internal error occurred",
	)
	w.Header().Set("Content-Type", "application/problem+json")
	body, _ := json.Marshal(pe)
	w.WriteHeader(pe.StatusCode())
	_, _ = w.Write(body)
}
