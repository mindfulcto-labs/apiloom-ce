package aiproxy

import (
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/router"
)

const (
	// DefaultMaxRequestBodyBytes is the maximum size of an incoming request body (10 MB).
	DefaultMaxRequestBodyBytes int64 = 10 << 20
)

// AIProxyConfig holds configuration for the AI proxy handler.
type AIProxyConfig struct {
	Models              []ai.Model      `json:"models"`
	DefaultStrategy     router.Strategy `json:"default_strategy"`
	FallbackEnabled     bool            `json:"fallback_enabled"`
	BudgetEnabled       bool            `json:"budget_enabled"`
	MaxRequestBodyBytes int64           `json:"max_request_body_bytes"`
}

// RouteAIConfig holds per-route AI configuration overrides.
type RouteAIConfig struct {
	Models        []string `json:"models"`
	Strategy      string   `json:"strategy"`
	FallbackOrder []string `json:"fallback_order"`
}
