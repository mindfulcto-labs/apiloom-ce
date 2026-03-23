package observability

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIMetrics_NewAIMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewAIMetrics(reg)
	require.NotNil(t, m)

	// Touch each metric so Gather() returns them.
	m.ObserveRequest("test", "test", "test", 1, 1, 0.001, time.Second)
	m.RecordFallback("a", "b")
	m.RecordProviderError("test", "err")
	m.UpdateBudgetUtilization("test", "daily", 0.5)

	families, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	expected := []string{
		"flexgw_llm_tokens_total",
		"flexgw_llm_request_cost_dollars",
		"flexgw_llm_request_duration_seconds",
		"flexgw_llm_budget_utilization_ratio",
		"flexgw_llm_fallback_total",
		"flexgw_llm_provider_errors_total",
	}
	for _, name := range expected {
		assert.True(t, names[name], "metric %s should be registered", name)
	}
}

func TestAIMetrics_ObserveRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		provider         string
		model            string
		consumer         string
		promptTokens     int64
		completionTokens int64
		cost             float64
		duration         time.Duration
		wantConsumer     string
	}{
		{
			name:             "normal request",
			provider:         "openai",
			model:            "gpt-4",
			consumer:         "abc-123",
			promptTokens:     100,
			completionTokens: 50,
			cost:             0.005,
			duration:         2 * time.Second,
			wantConsumer:     "abc-123",
		},
		{
			name:             "empty consumer defaults to anonymous",
			provider:         "anthropic",
			model:            "claude-3",
			consumer:         "",
			promptTokens:     200,
			completionTokens: 100,
			cost:             0.01,
			duration:         1 * time.Second,
			wantConsumer:     "anonymous",
		},
		{
			name:             "zero tokens",
			provider:         "openai",
			model:            "gpt-3.5",
			consumer:         "xyz-789",
			promptTokens:     0,
			completionTokens: 0,
			cost:             0,
			duration:         100 * time.Millisecond,
			wantConsumer:     "xyz-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			m := NewAIMetrics(reg)

			m.ObserveRequest(tt.provider, tt.model, tt.consumer, tt.promptTokens, tt.completionTokens, tt.cost, tt.duration)

			families, err := reg.Gather()
			require.NoError(t, err)

			familyMap := make(map[string]bool)
			for _, f := range families {
				familyMap[f.GetName()] = true
			}

			// Verify token counter was incremented (input direction).
			if tt.promptTokens > 0 {
				assert.True(t, familyMap["flexgw_llm_tokens_total"], "tokens_total should exist")
			}

			// Verify cost counter was incremented.
			if tt.cost > 0 {
				assert.True(t, familyMap["flexgw_llm_request_cost_dollars"], "cost_dollars should exist")
			}

			// Verify duration histogram was observed.
			assert.True(t, familyMap["flexgw_llm_request_duration_seconds"], "duration_seconds should exist")
		})
	}
}

func TestAIMetrics_ObserveRequest_ConsumerLabel(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewAIMetrics(reg)

	// Observe with empty consumer.
	m.ObserveRequest("openai", "gpt-4", "", 10, 5, 0.001, time.Second)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() == "flexgw_llm_tokens_total" {
			for _, metric := range f.GetMetric() {
				for _, lp := range metric.GetLabel() {
					if lp.GetName() == "consumer" {
						assert.Equal(t, "anonymous", lp.GetValue(), "empty consumer should default to anonymous")
					}
				}
			}
		}
	}
}

func TestAIMetrics_RecordFallback(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewAIMetrics(reg)

	m.RecordFallback("gpt-4", "gpt-3.5-turbo")

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, f := range families {
		if f.GetName() == "flexgw_llm_fallback_total" {
			found = true
			require.Len(t, f.GetMetric(), 1)
			metric := f.GetMetric()[0]
			assert.Equal(t, float64(1), metric.GetCounter().GetValue())

			labels := make(map[string]string)
			for _, lp := range metric.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			assert.Equal(t, "gpt-4", labels["primary_model"])
			assert.Equal(t, "gpt-3.5-turbo", labels["fallback_model"])
		}
	}
	assert.True(t, found, "fallback_total metric should be present")
}

func TestAIMetrics_RecordProviderError(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewAIMetrics(reg)

	m.RecordProviderError("openai", "rate_limit")
	m.RecordProviderError("openai", "rate_limit")
	m.RecordProviderError("anthropic", "timeout")

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, f := range families {
		if f.GetName() == "flexgw_llm_provider_errors_total" {
			found = true
			require.Len(t, f.GetMetric(), 2)

			for _, metric := range f.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["provider"] == "openai" && labels["error_type"] == "rate_limit" {
					assert.Equal(t, float64(2), metric.GetCounter().GetValue())
				}
				if labels["provider"] == "anthropic" && labels["error_type"] == "timeout" {
					assert.Equal(t, float64(1), metric.GetCounter().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "provider_errors_total metric should be present")
}

func TestAIMetrics_UpdateBudgetUtilization(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewAIMetrics(reg)

	m.UpdateBudgetUtilization("consumer-1", "daily", 0.75)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, f := range families {
		if f.GetName() == "flexgw_llm_budget_utilization_ratio" {
			found = true
			require.Len(t, f.GetMetric(), 1)
			metric := f.GetMetric()[0]
			assert.Equal(t, 0.75, metric.GetGauge().GetValue())

			labels := make(map[string]string)
			for _, lp := range metric.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			assert.Equal(t, "consumer-1", labels["consumer"])
			assert.Equal(t, "daily", labels["period"])
		}
	}
	assert.True(t, found, "budget_utilization_ratio metric should be present")

	// Update the same gauge to a new value.
	m.UpdateBudgetUtilization("consumer-1", "daily", 0.95)

	families, err = reg.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() == "flexgw_llm_budget_utilization_ratio" {
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, 0.95, f.GetMetric()[0].GetGauge().GetValue())
		}
	}
}
