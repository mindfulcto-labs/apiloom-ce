package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// AIMetrics holds all AI-specific Prometheus metric instruments.
// All methods are nil-safe: calling methods on a nil *AIMetrics is a no-op.
type AIMetrics struct {
	tokensTotal           *prometheus.CounterVec
	requestCostDollars    *prometheus.CounterVec
	requestDuration       *prometheus.HistogramVec
	budgetUtilization     *prometheus.GaugeVec
	fallbackTotal         *prometheus.CounterVec
	providerErrorsTotal   *prometheus.CounterVec
}

// NewAIMetrics creates and registers all AI-specific Prometheus metrics.
// Consumer labels use consumer IDs (UUIDs), never names or key prefixes.
// API keys and bearer tokens must never appear in metric labels.
func NewAIMetrics(reg prometheus.Registerer) *AIMetrics {
	m := &AIMetrics{
		tokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_llm_tokens_total",
				Help: "Total LLM tokens processed.",
			},
			[]string{"provider", "model", "direction", "consumer"},
		),
		requestCostDollars: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_llm_request_cost_dollars",
				Help: "Total estimated cost of LLM requests in dollars.",
			},
			[]string{"provider", "model", "consumer"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "flexgw_llm_request_duration_seconds",
				Help:    "LLM request duration in seconds.",
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"provider", "model"},
		),
		budgetUtilization: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "flexgw_llm_budget_utilization_ratio",
				Help: "Budget utilization ratio per consumer and period (0.0 to 1.0+).",
			},
			[]string{"consumer", "period"},
		),
		fallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_llm_fallback_total",
				Help: "Total number of fallback events between models.",
			},
			[]string{"primary_model", "fallback_model"},
		),
		providerErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flexgw_llm_provider_errors_total",
				Help: "Total number of provider errors by type.",
			},
			[]string{"provider", "error_type"},
		),
	}

	reg.MustRegister(
		m.tokensTotal,
		m.requestCostDollars,
		m.requestDuration,
		m.budgetUtilization,
		m.fallbackTotal,
		m.providerErrorsTotal,
	)

	return m
}

// ObserveRequest records tokens, cost, and duration for a completed AI request.
// Consumer uses "anonymous" if the provided consumer string is empty.
func (m *AIMetrics) ObserveRequest(provider, model, consumer string, promptTokens, completionTokens int64, cost float64, duration time.Duration) {
	if m == nil {
		return
	}
	if consumer == "" {
		consumer = "anonymous"
	}

	if promptTokens > 0 {
		m.tokensTotal.WithLabelValues(provider, model, "input", consumer).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		m.tokensTotal.WithLabelValues(provider, model, "output", consumer).Add(float64(completionTokens))
	}

	m.requestCostDollars.WithLabelValues(provider, model, consumer).Add(cost)
	m.requestDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
}

// RecordFallback increments the fallback counter when a request falls back
// from one model to another.
func (m *AIMetrics) RecordFallback(primaryModel, fallbackModel string) {
	if m == nil {
		return
	}
	m.fallbackTotal.WithLabelValues(primaryModel, fallbackModel).Inc()
}

// RecordProviderError increments the provider error counter.
func (m *AIMetrics) RecordProviderError(provider, errorType string) {
	if m == nil {
		return
	}
	m.providerErrorsTotal.WithLabelValues(provider, errorType).Inc()
}

// UpdateBudgetUtilization sets the budget utilization gauge for a consumer
// and period. Ratio is typically 0.0 to 1.0 but can exceed 1.0 for
// over-budget consumers.
func (m *AIMetrics) UpdateBudgetUtilization(consumer, period string, ratio float64) {
	if m == nil {
		return
	}
	m.budgetUtilization.WithLabelValues(consumer, period).Set(ratio)
}
