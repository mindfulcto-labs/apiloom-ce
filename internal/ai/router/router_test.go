package router

import (
	"testing"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		strategy Strategy
		wantErr  bool
	}{
		{name: "round-robin", strategy: StrategyRoundRobin, wantErr: false},
		{name: "cost-optimized", strategy: StrategyCostOptimized, wantErr: false},
		{name: "latency-optimized", strategy: StrategyLatencyOptimized, wantErr: false},
		{name: "capability-based", strategy: StrategyCapabilityBased, wantErr: false},
		{name: "weighted", strategy: StrategyWeighted, wantErr: false},
		{name: "unknown strategy", strategy: Strategy("unknown"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var opts []Option
			if tt.strategy == StrategyLatencyOptimized {
				opts = append(opts, WithLatencyTracker(&mockTracker{}))
			}
			r, err := NewRouter(tt.strategy, opts...)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, r)
			} else {
				require.NoError(t, err)
				require.NotNil(t, r)
			}
		})
	}
}

func TestRouterRouteNoModels(t *testing.T) {
	t.Parallel()
	strategies := []Strategy{
		StrategyRoundRobin,
		StrategyCostOptimized,
		StrategyCapabilityBased,
		StrategyWeighted,
	}
	for _, s := range strategies {
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			var opts []Option
			if s == StrategyLatencyOptimized {
				opts = append(opts, WithLatencyTracker(&mockTracker{}))
			}
			r, err := NewRouter(s, opts...)
			require.NoError(t, err)
			_, err = r.Route(&ai.ChatRequest{Model: "test"}, nil)
			require.ErrorIs(t, err, ErrNoModelsAvailable)
		})
	}
}

func TestRouterRouteAllInactive(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "openai", ModelName: "gpt-4", IsActive: false, Weight: 1},
		{Provider: "anthropic", ModelName: "claude", IsActive: false, Weight: 1},
	}
	r, err := NewRouter(StrategyRoundRobin)
	require.NoError(t, err)
	_, err = r.Route(&ai.ChatRequest{Model: "test"}, models)
	require.ErrorIs(t, err, ErrNoModelsAvailable)
}

// mockTracker implements LatencyTracker for testing.
type mockTracker struct {
	latencies map[string]int64 // key: "provider/model" -> ms
}

func (m *mockTracker) AverageLatencyMs(provider, model string) int64 {
	if m.latencies == nil {
		return 0
	}
	return m.latencies[provider+"/"+model]
}
