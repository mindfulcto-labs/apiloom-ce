package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAdapter creates an adapter pointing at a local httptest server,
// bypassing SSRF validation which blocks 127.0.0.1 in production.
func testAdapter(t *testing.T, baseURL, apiKey string) *Adapter {
	t.Helper()
	return newAdapter(provider.ProviderConfig{
		Name:    "openai",
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     provider.ProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: provider.ProviderConfig{
				Name:    "openai",
				BaseURL: "https://api.openai.com",
				APIKey:  "sk-test-key",
			},
			wantErr: false,
		},
		{
			name: "empty base URL",
			cfg: provider.ProviderConfig{
				Name:   "openai",
				APIKey: "sk-test-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, err := New(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, a)
		})
	}
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	temp := 0.7
	maxTok := 100

	tests := []struct {
		name string
		req  *ai.ChatRequest
	}{
		{
			name: "basic message",
			req: &ai.ChatRequest{
				Model: "gpt-4",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
		},
		{
			name: "with system message and tools",
			req: &ai.ChatRequest{
				Model: "gpt-4",
				Messages: []ai.ChatMessage{
					{Role: "system", Content: "You are helpful"},
					{Role: "user", Content: "Hi"},
				},
				Temperature: &temp,
				MaxTokens:   &maxTok,
				Tools: []ai.Tool{
					{Type: "function", Function: ai.ToolFunction{Name: "get_weather", Description: "Get weather"}},
				},
			},
		},
		{
			name: "streaming request adds stream_options",
			req: &ai.ChatRequest{
				Model: "gpt-4",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hello"},
				},
				Stream: true,
			},
		},
	}

	a := testAdapter(t, "https://api.openai.com", "sk-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := a.Translate(tt.req)
			require.NoError(t, err)

			b, ok := got.([]byte)
			require.True(t, ok, "Translate should return []byte")

			var decoded map[string]any
			require.NoError(t, json.Unmarshal(b, &decoded))

			assert.Equal(t, tt.req.Model, decoded["model"])

			if tt.req.Stream {
				so, ok := decoded["stream_options"].(map[string]any)
				require.True(t, ok, "streaming request should have stream_options")
				assert.Equal(t, true, so["include_usage"])
			}
		})
	}
}

func TestTranslateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resp     string
		model    string
		wantRole string
		wantText string
	}{
		{
			name: "standard response",
			resp: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1700000000,
				"model": "gpt-4",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hi there"}, "finish_reason": "stop"}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`,
			model:    "gpt-4",
			wantRole: "assistant",
			wantText: "Hi there",
		},
	}

	a := testAdapter(t, "https://api.openai.com", "sk-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := a.TranslateResponse([]byte(tt.resp), tt.model)
			require.NoError(t, err)
			require.Len(t, got.Choices, 1)
			assert.Equal(t, tt.wantRole, got.Choices[0].Message.Role)
			assert.Equal(t, "Hi there", got.Choices[0].Message.Content)
		})
	}
}

func TestCall(t *testing.T) {
	t.Parallel()

	t.Run("successful non-streaming call", func(t *testing.T) {
		t.Parallel()
		respBody := `{"id":"chatcmpl-123","choices":[{"message":{"role":"assistant","content":"Hi"}}]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(respBody))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-test-key")

		reqBody, _ := json.Marshal(map[string]any{"model": "gpt-4"})
		result, err := a.Call(context.Background(), reqBody, false)
		require.NoError(t, err)
		defer func() { _ = result.Body.Close() }()

		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.False(t, result.Streaming)

		body, _ := io.ReadAll(result.Body)
		assert.Contains(t, string(body), "chatcmpl-123")
	})

	t.Run("streaming call", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-test-key")

		reqBody, _ := json.Marshal(map[string]any{"model": "gpt-4", "stream": true})
		result, err := a.Call(context.Background(), reqBody, true)
		require.NoError(t, err)
		defer func() { _ = result.Body.Close() }()

		assert.True(t, result.Streaming)
	})

	t.Run("error response sanitized", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-test-key")

		reqBody, _ := json.Marshal(map[string]any{"model": "gpt-4"})
		_, err := a.Call(context.Background(), reqBody, false)
		require.Error(t, err)

		assert.NotContains(t, err.Error(), `"type":"rate_limit_error"`)
		assert.NotContains(t, err.Error(), "sk-test-key")
		assert.Contains(t, err.Error(), "Rate limit exceeded")
	})

	t.Run("500 error sanitized", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"Internal server error","type":"server_error"},"sensitive_data":"secret123"}`))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-test-key")

		reqBody, _ := json.Marshal(map[string]any{"model": "gpt-4"})
		_, err := a.Call(context.Background(), reqBody, false)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret123")
		assert.NotContains(t, err.Error(), "sensitive_data")
	})
}

func TestSSRFRejection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "loopback", baseURL: "http://127.0.0.1"},
		{name: "private 10.x", baseURL: "http://10.0.0.1"},
		{name: "private 192.168", baseURL: "http://192.168.1.1"},
		{name: "localhost", baseURL: "http://localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(provider.ProviderConfig{
				Name:    "openai",
				BaseURL: tt.baseURL,
				APIKey:  "sk-test",
			})
			require.Error(t, err)
		})
	}
}

func TestAPIKeyNotInErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	apiKey := "sk-super-secret-key-12345"
	a := testAdapter(t, srv.URL, apiKey)

	reqBody, _ := json.Marshal(map[string]any{"model": "gpt-4"})
	_, err := a.Call(context.Background(), reqBody, false)
	require.Error(t, err)

	errStr := err.Error()
	assert.NotContains(t, errStr, apiKey)
	assert.NotContains(t, errStr, strings.TrimPrefix(apiKey, "sk-"))
}

// Verify the adapter implements the Provider interface at compile time.
var _ provider.Provider = (*Adapter)(nil)
