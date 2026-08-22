package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
)

func TestCounter_EstimateTokens_OpenAI(t *testing.T) {
	t.Parallel()

	c := NewCounter()
	messages := []ai.ChatMessage{
		{Role: "user", Content: "Hello, world!"},
	}

	count, err := c.EstimateTokens("openai", messages)
	require.NoError(t, err)
	assert.Greater(t, count, int64(0))
}

func TestCounter_EstimateTokens_Anthropic(t *testing.T) {
	t.Parallel()

	c := NewCounter()
	messages := []ai.ChatMessage{
		{Role: "user", Content: "Hello, world!"},
	}

	count, err := c.EstimateTokens("anthropic", messages)
	require.NoError(t, err)
	// "Hello, world!" is 13 chars -> ceil(13/4) = 4 tokens
	assert.Equal(t, int64(4), count)
}

func TestCounter_EstimateTokens_Gemini(t *testing.T) {
	t.Parallel()

	c := NewCounter()
	messages := []ai.ChatMessage{
		{Role: "user", Content: "Hello, world!"},
	}

	count, err := c.EstimateTokens("gemini", messages)
	require.NoError(t, err)
	assert.Equal(t, int64(4), count)
}

func TestCounter_EstimateTokens_EmptyMessages(t *testing.T) {
	t.Parallel()

	c := NewCounter()
	count, err := c.EstimateTokens("openai", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestCounter_Reconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		estimated int64
		actual    ai.Usage
		wantDelta int64
	}{
		{
			name:      "exact match",
			estimated: 100,
			actual:    ai.Usage{TotalTokens: 100},
			wantDelta: 0,
		},
		{
			name:      "over-estimated",
			estimated: 120,
			actual:    ai.Usage{TotalTokens: 100},
			wantDelta: 20,
		},
		{
			name:      "under-estimated",
			estimated: 80,
			actual:    ai.Usage{TotalTokens: 100},
			wantDelta: -20,
		},
	}

	c := NewCounter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			delta := c.Reconcile(tt.estimated, tt.actual)
			assert.Equal(t, tt.wantDelta, delta)
		})
	}
}

func TestCounter_EstimateCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		model            ai.Model
		promptTokens     int64
		completionTokens int64
		wantCost         float64
	}{
		{
			name: "standard pricing",
			model: ai.Model{
				InputPricePerToken:  0.00003,
				OutputPricePerToken: 0.00006,
			},
			promptTokens:     1000,
			completionTokens: 500,
			wantCost:         0.06, // (1000 * 0.00003) + (500 * 0.00006) = 0.03 + 0.03
		},
		{
			name: "zero tokens",
			model: ai.Model{
				InputPricePerToken:  0.00003,
				OutputPricePerToken: 0.00006,
			},
			promptTokens:     0,
			completionTokens: 0,
			wantCost:         0.0,
		},
	}

	c := NewCounter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cost := c.EstimateCost(tt.model, tt.promptTokens, tt.completionTokens)
			assert.InDelta(t, tt.wantCost, cost, 0.0001)
		})
	}
}
