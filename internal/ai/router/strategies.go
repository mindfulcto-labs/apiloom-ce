package router

import (
	"crypto/rand"
	"math/big"
	"sync/atomic"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
)

// --- RoundRobin ---

// roundRobinRouter distributes requests evenly across active models using
// an atomic counter. Thread-safe without mutex per concurrency.md.
type roundRobinRouter struct {
	counter atomic.Uint64
}

func newRoundRobinRouter() *roundRobinRouter {
	return &roundRobinRouter{}
}

func (r *roundRobinRouter) Route(_ *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	active := filterActive(models)
	if len(active) == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}
	idx := r.counter.Add(1) - 1
	return active[idx%uint64(len(active))], nil
}

// --- CostOptimized ---

// costOptimizedRouter selects the cheapest active model by total per-token cost.
type costOptimizedRouter struct{}

func (r *costOptimizedRouter) Route(_ *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	active := filterActive(models)
	if len(active) == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}

	best := active[0]
	bestCost := best.InputPricePerToken + best.OutputPricePerToken
	for _, m := range active[1:] {
		cost := m.InputPricePerToken + m.OutputPricePerToken
		if cost < bestCost {
			best = m
			bestCost = cost
		}
	}
	return best, nil
}

// --- LatencyOptimized ---

// latencyOptimizedRouter selects the model with the lowest tracked average latency.
// When no latency data exists (all return 0), defaults to the first active model.
type latencyOptimizedRouter struct {
	tracker LatencyTracker
}

func (r *latencyOptimizedRouter) Route(_ *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	active := filterActive(models)
	if len(active) == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}

	best := active[0]
	bestLatency := r.tracker.AverageLatencyMs(best.Provider, best.ModelName)

	// If the first model has zero latency (no data), check if any model has data.
	// If none do, we return the first model as default.
	hasData := bestLatency > 0

	for _, m := range active[1:] {
		lat := r.tracker.AverageLatencyMs(m.Provider, m.ModelName)
		if lat > 0 {
			if !hasData || lat < bestLatency {
				best = m
				bestLatency = lat
				hasData = true
			}
		}
	}
	return best, nil
}

// --- CapabilityBased ---

// capabilityRouter filters models by required capability tags, then selects
// the first match. If no capabilities are required, selects the first active model.
type capabilityRouter struct{}

func (r *capabilityRouter) Route(req *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	active := filterActive(models)
	if len(active) == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}

	required := req.RequiredCapabilities
	if len(required) == 0 {
		return active[0], nil
	}

	for _, m := range active {
		if hasAllCapabilities(m, required) {
			return m, nil
		}
	}
	return ai.Model{}, ErrNoModelsAvailable
}

// hasAllCapabilities checks if a model has all required capability tags.
func hasAllCapabilities(m ai.Model, required []string) bool {
	caps := make(map[string]struct{}, len(m.Capabilities))
	for _, c := range m.Capabilities {
		caps[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := caps[r]; !ok {
			return false
		}
	}
	return true
}

// --- Weighted ---

// weightedRouter selects models proportional to their weight using crypto/rand.
// Models with weight 0 are never selected. Per security.md: crypto/rand only.
type weightedRouter struct{}

func (r *weightedRouter) Route(_ *ai.ChatRequest, models []ai.Model) (ai.Model, error) {
	active := filterActive(models)
	if len(active) == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}

	// Calculate total weight, filtering out zero-weight models.
	var totalWeight int64
	candidates := make([]ai.Model, 0, len(active))
	for _, m := range active {
		if m.Weight > 0 {
			totalWeight += int64(m.Weight)
			candidates = append(candidates, m)
		}
	}
	if totalWeight == 0 {
		return ai.Model{}, ErrNoModelsAvailable
	}

	// crypto/rand for weighted random selection.
	n, err := rand.Int(rand.Reader, big.NewInt(totalWeight))
	if err != nil {
		// Fallback to first candidate on crypto/rand failure (should never happen).
		return candidates[0], nil
	}
	pick := n.Int64()

	var cumulative int64
	for _, m := range candidates {
		cumulative += int64(m.Weight)
		if pick < cumulative {
			return m, nil
		}
	}

	// Should not reach here, but return last candidate as safety.
	return candidates[len(candidates)-1], nil
}
