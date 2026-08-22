package aiproxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/observability"
)

// FallbackChain tries models in order, skipping those whose provider
// returns errors. On success it returns the ProviderResult and the model used.
type FallbackChain struct {
	providers map[string]provider.Provider
	logger    *slog.Logger
	aiMetrics *observability.AIMetrics
}

// NewFallbackChain creates a new FallbackChain.
func NewFallbackChain(providers map[string]provider.Provider, logger *slog.Logger) *FallbackChain {
	return &FallbackChain{
		providers: providers,
		logger:    logger,
	}
}

// Execute tries each model in order, returning the first successful response.
// On all failures it returns a 502 ProblemError.
func (fc *FallbackChain) Execute(ctx context.Context, req *ai.ChatRequest, models []ai.Model) (*ai.ProviderResult, ai.Model, error) {
	var lastErr error
	var primaryModel string

	for i, model := range models {
		if i == 0 {
			primaryModel = model.ModelName
		}

		p, ok := fc.providers[model.Provider]
		if !ok {
			fc.logger.Warn("no provider registered for model, skipping",
				"provider", model.Provider,
				"model", model.ModelName,
			)
			lastErr = fmt.Errorf("no provider for %s", model.Provider)
			if fc.aiMetrics != nil {
				fc.aiMetrics.RecordProviderError(model.Provider, "not_registered")
			}
			continue
		}

		providerReq, err := p.Translate(req)
		if err != nil {
			fc.logger.Warn("translate failed, trying next model",
				"provider", model.Provider,
				"model", model.ModelName,
				"error", err,
			)
			lastErr = err
			if fc.aiMetrics != nil {
				fc.aiMetrics.RecordProviderError(model.Provider, "translate_error")
			}
			continue
		}

		result, err := p.Call(ctx, providerReq, req.Stream)
		if err != nil {
			fc.logger.Warn("provider call failed, trying next model",
				"provider", model.Provider,
				"model", model.ModelName,
				"error", err,
			)
			lastErr = err
			if fc.aiMetrics != nil {
				fc.aiMetrics.RecordProviderError(model.Provider, "call_error")
			}
			continue
		}

		// Record fallback if this is not the first (primary) model.
		if i > 0 && fc.aiMetrics != nil {
			fc.aiMetrics.RecordFallback(primaryModel, model.ModelName)
		}

		return result, model, nil
	}

	return nil, ai.Model{}, domain.NewProblemError(
		http.StatusBadGateway,
		"all_models_failed",
		fmt.Sprintf("all models failed: %v", lastErr),
	)
}
