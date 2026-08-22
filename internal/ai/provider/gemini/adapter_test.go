package gemini

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
func testAdapter(t *testing.T, baseURL, apiKey, authMode string) *Adapter {
	t.Helper()
	return newAdapter(provider.ProviderConfig{
		Name:    "gemini",
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, authMode)
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
				Name:    "gemini",
				BaseURL: "https://generativelanguage.googleapis.com",
				APIKey:  "AIza-test-key",
			},
			wantErr: false,
		},
		{
			name: "empty base URL",
			cfg: provider.ProviderConfig{
				Name:   "gemini",
				APIKey: "AIza-test-key",
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
			name: "basic message maps user role",
			req: &ai.ChatRequest{
				Model: "gemini-1.5-pro",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				contents := d["contents"].([]any)
				require.Len(t, contents, 1)
				c := contents[0].(map[string]any)
				assert.Equal(t, "user", c["role"])
				parts := c["parts"].([]any)
				require.Len(t, parts, 1)
				assert.Equal(t, "Hello", parts[0].(map[string]any)["text"])
			},
		},
		{
			name: "assistant role maps to model",
			req: &ai.ChatRequest{
				Model: "gemini-1.5-pro",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hi"},
					{Role: "assistant", Content: "Hello"},
					{Role: "user", Content: "How are you?"},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				contents := d["contents"].([]any)
				require.Len(t, contents, 3)
				assert.Equal(t, "user", contents[0].(map[string]any)["role"])
				assert.Equal(t, "model", contents[1].(map[string]any)["role"])
				assert.Equal(t, "user", contents[2].(map[string]any)["role"])
			},
		},
		{
			name: "system message extracts to systemInstruction",
			req: &ai.ChatRequest{
				Model: "gemini-1.5-pro",
				Messages: []ai.ChatMessage{
					{Role: "system", Content: "You are helpful"},
					{Role: "user", Content: "Hi"},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				si := d["systemInstruction"].(map[string]any)
				parts := si["parts"].([]any)
				require.Len(t, parts, 1)
				assert.Equal(t, "You are helpful", parts[0].(map[string]any)["text"])
				contents := d["contents"].([]any)
				require.Len(t, contents, 1, "system should not be in contents")
			},
		},
		{
			name: "generation config maps temperature and maxOutputTokens",
			req: &ai.ChatRequest{
				Model: "gemini-1.5-pro",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Hi"},
				},
				Temperature: &temp,
				MaxTokens:   &maxTok,
			},
			checkFn: func(t *testing.T, d map[string]any) {
				gc := d["generationConfig"].(map[string]any)
				assert.Equal(t, 0.7, gc["temperature"])
				assert.Equal(t, float64(200), gc["maxOutputTokens"])
			},
		},
		{
			name: "tools map to functionDeclarations",
			req: &ai.ChatRequest{
				Model: "gemini-1.5-pro",
				Messages: []ai.ChatMessage{
					{Role: "user", Content: "Weather?"},
				},
				Tools: []ai.Tool{
					{
						Type: "function",
						Function: ai.ToolFunction{
							Name:        "get_weather",
							Description: "Get weather",
							Parameters:  map[string]any{"type": "object"},
						},
					},
				},
			},
			checkFn: func(t *testing.T, d map[string]any) {
				tools := d["tools"].([]any)
				require.Len(t, tools, 1)
				tool := tools[0].(map[string]any)
				fds := tool["functionDeclarations"].([]any)
				require.Len(t, fds, 1)
				assert.Equal(t, "get_weather", fds[0].(map[string]any)["name"])
			},
		},
	}

	a := testAdapter(t, "https://generativelanguage.googleapis.com", "test-key", "apikey")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := a.Translate(tt.req)
			require.NoError(t, err)

			b, ok := got.(geminiCallPayload)
			require.True(t, ok)

			// Re-marshal to inspect as map
			jb, err := json.Marshal(b.body)
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(jb, &decoded))

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
				"candidates": [{
					"content": {"role": "model", "parts": [{"text": "Hello there"}]},
					"finishReason": "STOP"
				}],
				"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
			}`,
			model:         "gemini-1.5-pro",
			wantContent:   "Hello there",
			wantFinish:    "stop",
			wantPromptTok: 10,
			wantCompTok:   5,
		},
		{
			name: "max tokens finish reason",
			resp: `{
				"candidates": [{
					"content": {"role": "model", "parts": [{"text": "Truncated"}]},
					"finishReason": "MAX_TOKENS"
				}],
				"usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 50, "totalTokenCount": 150}
			}`,
			model:         "gemini-1.5-pro",
			wantContent:   "Truncated",
			wantFinish:    "length",
			wantPromptTok: 100,
			wantCompTok:   50,
		},
		{
			name: "safety stop",
			resp: `{
				"candidates": [{
					"content": {"role": "model", "parts": [{"text": ""}]},
					"finishReason": "SAFETY"
				}],
				"usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 0, "totalTokenCount": 5}
			}`,
			model:         "gemini-1.5-pro",
			wantContent:   "",
			wantFinish:    "content_filter",
			wantPromptTok: 5,
			wantCompTok:   0,
		},
	}

	a := testAdapter(t, "https://generativelanguage.googleapis.com", "test-key", "apikey")

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
		})
	}
}

func TestCallNonStreaming(t *testing.T) {
	t.Parallel()

	respBody := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"}]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/v1beta/models/gemini-1.5-pro:generateContent")
		assert.Contains(t, r.URL.RawQuery, "key=test-api-key")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(respBody))
	}))
	defer srv.Close()

	a := testAdapter(t, srv.URL, "test-api-key", "apikey")

	payload := geminiCallPayload{
		model:     "gemini-1.5-pro",
		body:      map[string]any{"contents": []any{}},
		streaming: false,
	}
	result, err := a.Call(context.Background(), payload, false)
	require.NoError(t, err)
	defer result.Body.Close()

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.False(t, result.Streaming)

	body, _ := io.ReadAll(result.Body)
	assert.Contains(t, string(body), "Hi")
}

func TestCallStreaming(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1beta/models/gemini-1.5-pro:streamGenerateContent")
		assert.Contains(t, r.URL.RawQuery, "alt=sse")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}]}\n\n"))
	}))
	defer srv.Close()

	a := testAdapter(t, srv.URL, "test-api-key", "apikey")

	payload := geminiCallPayload{
		model:     "gemini-1.5-pro",
		body:      map[string]any{"contents": []any{}},
		streaming: true,
	}
	result, err := a.Call(context.Background(), payload, true)
	require.NoError(t, err)
	defer result.Body.Close()

	assert.True(t, result.Streaming)
}

func TestCallBearerAuth(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-bearer-token", r.Header.Get("Authorization"))
		assert.NotContains(t, r.URL.RawQuery, "key=")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	a := testAdapter(t, srv.URL, "test-bearer-token", "bearer")

	payload := geminiCallPayload{
		model:     "gemini-1.5-pro",
		body:      map[string]any{"contents": []any{}},
		streaming: false,
	}
	result, err := a.Call(context.Background(), payload, false)
	require.NoError(t, err)
	defer result.Body.Close()

	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestCallErrorSanitized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED","code":429,"sensitive":"secret"}}`))
	}))
	defer srv.Close()

	a := testAdapter(t, srv.URL, "test-api-key", "apikey")

	payload := geminiCallPayload{
		model:     "gemini-1.5-pro",
		body:      map[string]any{"contents": []any{}},
		streaming: false,
	}
	_, err := a.Call(context.Background(), payload, false)
	require.Error(t, err)

	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "RESOURCE_EXHAUSTED")
	assert.NotContains(t, err.Error(), "test-api-key")
	assert.Contains(t, err.Error(), "Quota exceeded")
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
				Name:    "gemini",
				BaseURL: tt.baseURL,
				APIKey:  "test-key",
			})
			require.Error(t, err)
		})
	}
}

func TestAPIKeyNotInErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"API key invalid"}}`))
	}))
	defer srv.Close()

	apiKey := "AIza-super-secret-key-12345"
	a := testAdapter(t, srv.URL, apiKey, "apikey")

	payload := geminiCallPayload{
		model:     "gemini-1.5-pro",
		body:      map[string]any{"contents": []any{}},
		streaming: false,
	}
	_, err := a.Call(context.Background(), payload, false)
	require.Error(t, err)

	errStr := err.Error()
	assert.NotContains(t, errStr, apiKey)
	assert.NotContains(t, errStr, strings.TrimPrefix(apiKey, "AIza-"))
}

// Verify compile-time interface compliance.
var _ provider.Provider = (*Adapter)(nil)
