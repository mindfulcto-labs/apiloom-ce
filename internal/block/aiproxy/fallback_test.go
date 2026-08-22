package aiproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	translateFn         func(req *ai.ChatRequest) (any, error)
	translateResponseFn func(providerResp []byte, model string) (*ai.ChatResponse, error)
	callFn              func(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error)
}

func (m *mockProvider) Translate(req *ai.ChatRequest) (any, error) {
	if m.translateFn != nil {
		return m.translateFn(req)
	}
	b, _ := json.Marshal(req)
	return b, nil
}

func (m *mockProvider) TranslateResponse(providerResp []byte, model string) (*ai.ChatResponse, error) {
	if m.translateResponseFn != nil {
		return m.translateResponseFn(providerResp, model)
	}
	var resp ai.ChatResponse
	err := json.Unmarshal(providerResp, &resp)
	return &resp, err
}

func (m *mockProvider) Call(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error) {
	if m.callFn != nil {
		return m.callFn(ctx, providerReq, streaming)
	}
	return nil, fmt.Errorf("mock: no callFn set")
}

func newSuccessProvider(respBody string, statusCode int) provider.Provider {
	return &mockProvider{
		callFn: func(_ context.Context, _ any, _ bool) (*ai.ProviderResult, error) {
			return &ai.ProviderResult{
				Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
				StatusCode: statusCode,
				Streaming:  false,
			}, nil
		},
	}
}

func newFailProvider(statusCode int) provider.Provider {
	return &mockProvider{
		callFn: func(_ context.Context, _ any, _ bool) (*ai.ProviderResult, error) {
			return nil, fmt.Errorf("provider error: status %d", statusCode)
		},
	}
}

func TestFallbackChain(t *testing.T) {
	t.Parallel()

	validResp := `{"id":"resp-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`

	t.Run("first model succeeds", func(t *testing.T) {
		t.Parallel()
		providers := map[string]provider.Provider{
			"openai": newSuccessProvider(validResp, 200),
		}
		models := []ai.Model{
			{Provider: "openai", ModelName: "gpt-4", IsActive: true},
		}

		fc := NewFallbackChain(providers, newDiscardLogger())
		req := &ai.ChatRequest{Model: "gpt-4", Messages: []ai.ChatMessage{{Role: "user", Content: "Hi"}}}
		result, modelUsed, err := fc.Execute(context.Background(), req, models)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "openai", modelUsed.Provider)
	})

	t.Run("fallback to second on first failure", func(t *testing.T) {
		t.Parallel()
		providers := map[string]provider.Provider{
			"openai":    newFailProvider(500),
			"anthropic": newSuccessProvider(validResp, 200),
		}
		models := []ai.Model{
			{Provider: "openai", ModelName: "gpt-4", IsActive: true},
			{Provider: "anthropic", ModelName: "claude-3", IsActive: true},
		}

		fc := NewFallbackChain(providers, newDiscardLogger())
		req := &ai.ChatRequest{Model: "gpt-4", Messages: []ai.ChatMessage{{Role: "user", Content: "Hi"}}}
		result, modelUsed, err := fc.Execute(context.Background(), req, models)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "anthropic", modelUsed.Provider)
	})

	t.Run("all models fail returns error", func(t *testing.T) {
		t.Parallel()
		providers := map[string]provider.Provider{
			"openai":    newFailProvider(500),
			"anthropic": newFailProvider(503),
		}
		models := []ai.Model{
			{Provider: "openai", ModelName: "gpt-4", IsActive: true},
			{Provider: "anthropic", ModelName: "claude-3", IsActive: true},
		}

		fc := NewFallbackChain(providers, newDiscardLogger())
		req := &ai.ChatRequest{Model: "gpt-4", Messages: []ai.ChatMessage{{Role: "user", Content: "Hi"}}}
		result, _, err := fc.Execute(context.Background(), req, models)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "all models failed")
	})

	t.Run("skips models with no provider", func(t *testing.T) {
		t.Parallel()
		providers := map[string]provider.Provider{
			"anthropic": newSuccessProvider(validResp, 200),
		}
		models := []ai.Model{
			{Provider: "missing", ModelName: "x", IsActive: true},
			{Provider: "anthropic", ModelName: "claude-3", IsActive: true},
		}

		fc := NewFallbackChain(providers, newDiscardLogger())
		req := &ai.ChatRequest{Model: "x", Messages: []ai.ChatMessage{{Role: "user", Content: "Hi"}}}
		result, modelUsed, err := fc.Execute(context.Background(), req, models)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "anthropic", modelUsed.Provider)
	})
}
