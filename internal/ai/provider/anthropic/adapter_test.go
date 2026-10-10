package anthropic

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
		Name:    "anthropic",
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
				Name:    "anthropic",
				BaseURL: "https://api.anthropic.com",
				APIKey:  "sk-ant-test",
			},
			wantErr: false,
		},
		{
			name: "empty base URL",
			cfg: provider.ProviderConfig{
				Name:   "anthropic",
				APIKey: "sk-ant-test",
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
	maxTok := 200

	tests := []struct {
		name    string
		req     *ai.ChatRequest
		checkFn func(t *testing.T, decoded map[string]any)
	}{
		{
			name: "basic message without system",
			req: &ai.ChatRequest{
				Model: "claude-3-sonnet-20240229",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				_, hasSystem := d["system"]
				assert.False(t, hasSystem)
				assert.Equal(t, float64(4096), d["max_tokens"])
				msgs := d["messages"].([]any)
				assert.Len(t, msgs, 1)
			},
		},
		{
			name: "extracts system message to top-level field",
			req: &ai.ChatRequest{
				Model: "claude-3-sonnet-20240229",
				Messages: []ai.ChatMessage{
					{Role: "system", Content: "You are helpful"},
					{Role: "user", Content: "Hi"},
				},
				Temperature: &temp,
				MaxTokens:   &maxTok,
			},
			checkFn: func(t *testing.T, d map[string]any) {
				assert.Equal(t, "You are helpful", d["system"])
				assert.Equal(t, float64(200), d["max_tokens"])
				assert.Equal(t, 0.7, d["temperature"])
				msgs := d["messages"].([]any)
				assert.Len(t, msgs, 1, "system message should be removed from messages array")
				firstMsg := msgs[0].(map[string]any)
				assert.Equal(t, "user", firstMsg["role"])
			},
		},
		{
			name: "multiple system messages concatenated",
			req: &ai.ChatRequest{
				Model: "claude-3-sonnet-20240229",
				Messages: []ai.ChatMessage{
					{Role: "system", Content: "Rule 1"},
					{Role: "system", Content: "Rule 2"},
					{Role: "user", Content: "Hi"},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				assert.Equal(t, "Rule 1\n\nRule 2", d["system"])
				msgs := d["messages"].([]any)
				assert.Len(t, msgs, 1)
			},
		},
		{
			name: "with tools",
			req: &ai.ChatRequest{
				Model: "claude-3-sonnet-20240229",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "What's the weather?"},
				},
				Tools: []ai.Tool{
					{
						Type: "function",
						Function: ai.ToolFunction{
							Name:        "get_weather",
							Description: "Get weather for a location",
							Parameters:  map[string]any{"type": "object"},
						},
					},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				tools := d["tools"].([]any)
				assert.Len(t, tools, 1)
				tool := tools[0].(map[string]any)
				assert.Equal(t, "get_weather", tool["name"])
			},
		},
		{
			name: "tool_choice auto string",
			req: &ai.ChatRequest{
				Model: "claude-3-sonnet-20240229",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hi"},
				},
				ToolChoice: "auto",
			},
			checkFn: func(t *testing.T, d map[string]any) {
				tc := d["tool_choice"].(map[string]any)
				assert.Equal(t, "auto", tc["type"])
			},
		},
	}

	a := testAdapter(t, "https://api.anthropic.com", "sk-ant-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := a.Translate(tt.req)
			require.NoError(t, err)

			b, ok := got.([]byte)
			require.True(t, ok)

			var decoded map[string]any
			require.NoError(t, json.Unmarshal(b, &decoded))

			assert.Equal(t, "claude-3-sonnet-20240229", decoded["model"])
			tt.checkFn(t, decoded)
		})
	}
}

func TestTranslateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		resp          string
		model         string
		wantContent   string
		wantFinish    string
		wantPromptTok int64
		wantCompTok   int64
	}{
		{
			name: "standard text response",
			resp: `{
				"id": "msg_123",
				"type": "message",
				"role": "assistant",
				"content": [{"type": "text", "text": "Hello there"}],
				"model": "claude-3-sonnet-20240229",
				"stop_reason": "end_turn",
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`,
			model:         "claude-3-sonnet-20240229",
			wantContent:   "Hello there",
			wantFinish:    "stop",
			wantPromptTok: 10,
			wantCompTok:   5,
		},
		{
			name: "max_tokens stop reason",
			resp: `{
				"id": "msg_456",
				"type": "message",
				"role": "assistant",
				"content": [{"type": "text", "text": "Truncated"}],
				"model": "claude-3-sonnet-20240229",
				"stop_reason": "max_tokens",
				"usage": {"input_tokens": 100, "output_tokens": 50}
			}`,
			model:         "claude-3-sonnet-20240229",
			wantContent:   "Truncated",
			wantFinish:    "length",
			wantPromptTok: 100,
			wantCompTok:   50,
		},
		{
			name: "tool_use response",
			resp: `{
				"id": "msg_789",
				"type": "message",
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Let me check"},
					{"type": "tool_use", "id": "toolu_01", "name": "get_weather", "input": {"location": "NYC"}}
				],
				"model": "claude-3-sonnet-20240229",
				"stop_reason": "tool_use",
				"usage": {"input_tokens": 20, "output_tokens": 30}
			}`,
			model:         "claude-3-sonnet-20240229",
			wantContent:   "Let me check",
			wantFinish:    "tool_calls",
			wantPromptTok: 20,
			wantCompTok:   30,
		},
	}

	a := testAdapter(t, "https://api.anthropic.com", "sk-ant-test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := a.TranslateResponse([]byte(tt.resp), tt.model)
			require.NoError(t, err)

			require.Len(t, got.Choices, 1)
			assert.Equal(t, "assistant", got.Choices[0].Message.Role)
			assert.Equal(t, tt.wantContent, got.Choices[0].Message.Content)
			assert.Equal(t, tt.wantFinish, got.Choices[0].FinishReason)
			assert.Equal(t, tt.wantPromptTok, got.Usage.PromptTokens)
			assert.Equal(t, tt.wantCompTok, got.Usage.CompletionTokens)
			assert.Equal(t, tt.wantPromptTok+tt.wantCompTok, got.Usage.TotalTokens)

			if tt.name == "tool_use response" {
				require.Len(t, got.Choices[0].Message.ToolCalls, 1)
				tc := got.Choices[0].Message.ToolCalls[0]
				assert.Equal(t, "toolu_01", tc.ID)
				assert.Equal(t, "get_weather", tc.Function.Name)
				assert.Contains(t, tc.Function.Arguments, "NYC")
			}
		})
	}
}

func TestCall(t *testing.T) {
	t.Parallel()

	t.Run("successful non-streaming call", func(t *testing.T) {
		t.Parallel()
		respBody := `{"id":"msg_123","content":[{"type":"text","text":"Hi"}]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/v1/messages", r.URL.Path)
			assert.Equal(t, "sk-ant-test", r.Header.Get("x-api-key"))
			assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(respBody))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-ant-test")

		reqBody, _ := json.Marshal(map[string]any{"model": "claude-3-sonnet"})
		result, err := a.Call(context.Background(), reqBody, false)
		require.NoError(t, err)
		defer func() { _ = result.Body.Close() }()

		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.False(t, result.Streaming)

		body, _ := io.ReadAll(result.Body)
		assert.Contains(t, string(body), "msg_123")
	})

	t.Run("streaming call", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"delta\":{\"text\":\"Hi\"}}\n\n"))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-ant-test")

		reqBody, _ := json.Marshal(map[string]any{"model": "claude-3-sonnet"})
		result, err := a.Call(context.Background(), reqBody, true)
		require.NoError(t, err)
		defer func() { _ = result.Body.Close() }()

		assert.True(t, result.Streaming)
	})

	t.Run("error response sanitized", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited"}}`))
		}))
		defer srv.Close()

		a := testAdapter(t, srv.URL, "sk-ant-test")

		reqBody, _ := json.Marshal(map[string]any{"model": "claude-3-sonnet"})
		_, err := a.Call(context.Background(), reqBody, false)
		require.Error(t, err)

		assert.NotContains(t, err.Error(), `"type":"rate_limit_error"`)
		assert.NotContains(t, err.Error(), "sk-ant-test")
		assert.Contains(t, err.Error(), "Rate limited")
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
		{name: "localhost", baseURL: "http://localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(provider.ProviderConfig{
				Name:    "anthropic",
				BaseURL: tt.baseURL,
				APIKey:  "sk-ant-test",
			})
			require.Error(t, err)
		})
	}
}

func TestAPIKeyNotInErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	apiKey := "sk-ant-super-secret-12345"
	a := testAdapter(t, srv.URL, apiKey)

	reqBody, _ := json.Marshal(map[string]any{"model": "claude-3-sonnet"})
	_, err := a.Call(context.Background(), reqBody, false)
	require.Error(t, err)

	errStr := err.Error()
	assert.NotContains(t, errStr, apiKey)
	assert.NotContains(t, errStr, strings.TrimPrefix(apiKey, "sk-ant-"))
}

// Verify compile-time interface compliance.
var _ provider.Provider = (*Adapter)(nil)
