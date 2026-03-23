package token

import (
	"fmt"
	"math"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
)

// Counter provides token estimation, reconciliation, and cost calculation.
type Counter struct{}

// NewCounter creates a new Counter instance.
func NewCounter() *Counter {
	return &Counter{}
}

// EstimateTokens estimates the token count for the given messages using
// provider-specific counting: tiktoken for OpenAI, character estimation for others.
func (c *Counter) EstimateTokens(provider string, messages []ai.ChatMessage) (int64, error) {
	if len(messages) == 0 {
		return 0, nil
	}

	switch provider {
	case "openai":
		return tiktokenCountMessages(messages)
	case "anthropic", "gemini":
		return c.charEstimateMessages(messages), nil
	default:
		return 0, fmt.Errorf("unsupported provider for token estimation: %s", provider)
	}
}

// charEstimateMessages estimates tokens for all messages using ~4 chars/token.
func (c *Counter) charEstimateMessages(messages []ai.ChatMessage) int64 {
	var total int64
	for _, msg := range messages {
		content := extractContent(msg.Content)
		if content == "" {
			continue
		}
		total += int64(math.Ceil(float64(len(content)) / 4.0))
	}
	return total
}

// Reconcile returns the delta between estimated and actual token usage.
// Positive means over-estimated, negative means under-estimated.
func (c *Counter) Reconcile(estimated int64, actual ai.Usage) int64 {
	return estimated - actual.TotalTokens
}

// EstimateCost calculates the USD cost based on model pricing and token counts.
func (c *Counter) EstimateCost(model ai.Model, promptTokens, completionTokens int64) float64 {
	inputCost := float64(promptTokens) * model.InputPricePerToken
	outputCost := float64(completionTokens) * model.OutputPricePerToken
	return inputCost + outputCost
}
