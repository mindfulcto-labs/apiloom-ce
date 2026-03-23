// Package router provides model routing strategies for the AI gateway.
// Each strategy implements the Router interface to select an appropriate
// model from a pool of available models based on different criteria.
package router

import (
	"errors"
	"fmt"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
)

// ErrNoModelsAvailable is returned when no active models match the routing criteria.
var ErrNoModelsAvailable = errors.New("no models available")

// Strategy identifies a routing algorithm.
type Strategy string

const (
	StrategyRoundRobin        Strategy = "round-robin"
	StrategyCostOptimized     Strategy = "cost-optimized"
	StrategyLatencyOptimized  Strategy = "latency-optimized"
	StrategyCapabilityBased   Strategy = "capability-based"
	StrategyWeighted          Strategy = "weighted"
)

// Router selects a model from a pool of candidates for a given request.
type Router interface {
	Route(req *ai.ChatRequest, models []ai.Model) (ai.Model, error)
}

// LatencyTracker provides average latency data for model selection.
type LatencyTracker interface {
	AverageLatencyMs(provider, model string) int64
}

// Option configures a Router during construction.
type Option func(*options)

type options struct {
	latencyTracker LatencyTracker
}

// WithLatencyTracker sets the latency tracker for LatencyOptimized strategy.
func WithLatencyTracker(lt LatencyTracker) Option {
	return func(o *options) {
		o.latencyTracker = lt
	}
}

// NewRouter creates a Router for the given strategy.
func NewRouter(strategy Strategy, opts ...Option) (Router, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	switch strategy {
	case StrategyRoundRobin:
		return newRoundRobinRouter(), nil
	case StrategyCostOptimized:
		return &costOptimizedRouter{}, nil
	case StrategyLatencyOptimized:
		if o.latencyTracker == nil {
			return &latencyOptimizedRouter{tracker: noopTracker{}}, nil
		}
		return &latencyOptimizedRouter{tracker: o.latencyTracker}, nil
	case StrategyCapabilityBased:
		return &capabilityRouter{}, nil
	case StrategyWeighted:
		return &weightedRouter{}, nil
	default:
		return nil, fmt.Errorf("unknown routing strategy: %s", string(strategy))
	}
}

// filterActive returns only active models from the input slice.
func filterActive(models []ai.Model) []ai.Model {
	active := make([]ai.Model, 0, len(models))
	for _, m := range models {
		if m.IsActive {
			active = append(active, m)
		}
	}
	return active
}

// noopTracker returns 0 for all latency queries.
type noopTracker struct{}

func (noopTracker) AverageLatencyMs(string, string) int64 { return 0 }
