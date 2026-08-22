package router

import (
	"testing"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RoundRobin ---

func TestRoundRobinDistribution(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "m1", IsActive: true, Weight: 1},
		{Provider: "b", ModelName: "m2", IsActive: true, Weight: 1},
		{Provider: "c", ModelName: "m3", IsActive: true, Weight: 1},
	}
	r, err := NewRouter(StrategyRoundRobin)
	require.NoError(t, err)

	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		m, err := r.Route(&ai.ChatRequest{}, models)
		require.NoError(t, err)
		counts[m.ModelName]++
	}
	assert.Equal(t, 2, counts["m1"])
	assert.Equal(t, 2, counts["m2"])
	assert.Equal(t, 2, counts["m3"])
}

func TestRoundRobinSkipsInactive(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "m1", IsActive: false, Weight: 1},
		{Provider: "b", ModelName: "m2", IsActive: true, Weight: 1},
		{Provider: "c", ModelName: "m3", IsActive: true, Weight: 1},
	}
	r, err := NewRouter(StrategyRoundRobin)
	require.NoError(t, err)

	for i := 0; i < 4; i++ {
		m, err := r.Route(&ai.ChatRequest{}, models)
		require.NoError(t, err)
		assert.True(t, m.IsActive)
	}
}

func TestRoundRobinSingleModel(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "m1", IsActive: true, Weight: 1},
	}
	r, err := NewRouter(StrategyRoundRobin)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		m, err := r.Route(&ai.ChatRequest{}, models)
		require.NoError(t, err)
		assert.Equal(t, "m1", m.ModelName)
	}
}

// --- CostOptimized ---

func TestCostOptimizedSelectsCheapest(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "expensive", IsActive: true, InputPricePerToken: 0.03, OutputPricePerToken: 0.06, Weight: 1},
		{Provider: "b", ModelName: "cheap", IsActive: true, InputPricePerToken: 0.001, OutputPricePerToken: 0.002, Weight: 1},
		{Provider: "c", ModelName: "mid", IsActive: true, InputPricePerToken: 0.01, OutputPricePerToken: 0.02, Weight: 1},
	}
	r, err := NewRouter(StrategyCostOptimized)
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "cheap", m.ModelName)
}

func TestCostOptimizedSkipsInactive(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "cheapest-inactive", IsActive: false, InputPricePerToken: 0.0001, OutputPricePerToken: 0.0001, Weight: 1},
		{Provider: "b", ModelName: "active", IsActive: true, InputPricePerToken: 0.01, OutputPricePerToken: 0.02, Weight: 1},
	}
	r, err := NewRouter(StrategyCostOptimized)
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "active", m.ModelName)
}

// --- LatencyOptimized ---

func TestLatencyOptimizedSelectsLowest(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "slow", IsActive: true, Weight: 1},
		{Provider: "b", ModelName: "fast", IsActive: true, Weight: 1},
		{Provider: "c", ModelName: "medium", IsActive: true, Weight: 1},
	}
	tracker := &mockTracker{latencies: map[string]int64{
		"a/slow":   500,
		"b/fast":   50,
		"c/medium": 200,
	}}
	r, err := NewRouter(StrategyLatencyOptimized, WithLatencyTracker(tracker))
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "fast", m.ModelName)
}

func TestLatencyOptimizedDefaultsToFirstWhenNoData(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "first", IsActive: true, Weight: 1},
		{Provider: "b", ModelName: "second", IsActive: true, Weight: 1},
	}
	tracker := &mockTracker{} // no latency data
	r, err := NewRouter(StrategyLatencyOptimized, WithLatencyTracker(tracker))
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "first", m.ModelName)
}

// --- CapabilityBased ---

func TestCapabilityBasedFiltersModels(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "no-vision", IsActive: true, Capabilities: []string{"chat"}, Weight: 1},
		{Provider: "b", ModelName: "has-vision", IsActive: true, Capabilities: []string{"chat", "vision"}, Weight: 1},
		{Provider: "c", ModelName: "also-vision", IsActive: true, Capabilities: []string{"vision", "chat"}, Weight: 1},
	}
	r, err := NewRouter(StrategyCapabilityBased)
	require.NoError(t, err)

	req := &ai.ChatRequest{}
	req.RequiredCapabilities = []string{"vision"}

	m, err := r.Route(req, models)
	require.NoError(t, err)
	assert.Equal(t, "has-vision", m.ModelName)
}

func TestCapabilityBasedNoMatchReturnsError(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "chat-only", IsActive: true, Capabilities: []string{"chat"}, Weight: 1},
	}
	r, err := NewRouter(StrategyCapabilityBased)
	require.NoError(t, err)

	req := &ai.ChatRequest{}
	req.RequiredCapabilities = []string{"vision"}

	_, err = r.Route(req, models)
	require.ErrorIs(t, err, ErrNoModelsAvailable)
}

func TestCapabilityBasedNoFilterSelectsFirst(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "first", IsActive: true, Capabilities: []string{"chat"}, Weight: 1},
		{Provider: "b", ModelName: "second", IsActive: true, Capabilities: []string{"vision"}, Weight: 1},
	}
	r, err := NewRouter(StrategyCapabilityBased)
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "first", m.ModelName)
}

// --- Weighted ---

func TestWeightedNeverSelectsZeroWeight(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "zero", IsActive: true, Weight: 0},
		{Provider: "b", ModelName: "nonzero", IsActive: true, Weight: 10},
	}
	r, err := NewRouter(StrategyWeighted)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		m, err := r.Route(&ai.ChatRequest{}, models)
		require.NoError(t, err)
		assert.Equal(t, "nonzero", m.ModelName)
	}
}

func TestWeightedProportionalDistribution(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "heavy", IsActive: true, Weight: 80},
		{Provider: "b", ModelName: "light", IsActive: true, Weight: 20},
	}
	r, err := NewRouter(StrategyWeighted)
	require.NoError(t, err)

	counts := map[string]int{}
	iterations := 1000
	for i := 0; i < iterations; i++ {
		m, err := r.Route(&ai.ChatRequest{}, models)
		require.NoError(t, err)
		counts[m.ModelName]++
	}

	// heavy should get ~80% (800), light ~20% (200), allow 20% tolerance
	heavyRatio := float64(counts["heavy"]) / float64(iterations)
	assert.InDelta(t, 0.80, heavyRatio, 0.10, "heavy model should get ~80%% of requests, got %.2f%%", heavyRatio*100)
}

func TestWeightedAllZeroWeight(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "z1", IsActive: true, Weight: 0},
		{Provider: "b", ModelName: "z2", IsActive: true, Weight: 0},
	}
	r, err := NewRouter(StrategyWeighted)
	require.NoError(t, err)

	_, err = r.Route(&ai.ChatRequest{}, models)
	require.ErrorIs(t, err, ErrNoModelsAvailable)
}

func TestWeightedSkipsInactive(t *testing.T) {
	t.Parallel()
	models := []ai.Model{
		{Provider: "a", ModelName: "inactive-heavy", IsActive: false, Weight: 100},
		{Provider: "b", ModelName: "active-light", IsActive: true, Weight: 1},
	}
	r, err := NewRouter(StrategyWeighted)
	require.NoError(t, err)

	m, err := r.Route(&ai.ChatRequest{}, models)
	require.NoError(t, err)
	assert.Equal(t, "active-light", m.ModelName)
}
