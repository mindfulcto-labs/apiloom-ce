package aiproxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResponseWriter implements http.ResponseWriter and http.Flusher for testing.
// httptest.ResponseRecorder does NOT implement http.Flusher, so we need a custom one.
type testResponseWriter struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
	flushed    int
}

func newTestResponseWriter() *testResponseWriter {
	return &testResponseWriter{header: make(http.Header)}
}

func (w *testResponseWriter) Header() http.Header        { return w.header }
func (w *testResponseWriter) WriteHeader(statusCode int)  { w.statusCode = statusCode }
func (w *testResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *testResponseWriter) Flush()                      { w.flushed++ }

// Unwrap allows http.NewResponseController to find the Flusher.
func (w *testResponseWriter) Unwrap() http.ResponseWriter { return nil }

func TestTokenAccumulator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		provider         string
		events           []string
		wantPrompt       int64
		wantCompletion   int64
	}{
		{
			name:     "openai usage in final chunk",
			provider: "openai",
			events: []string{
				`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
				`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			},
			wantPrompt:     10,
			wantCompletion: 5,
		},
		{
			name:     "anthropic usage from message_start and message_delta",
			provider: "anthropic",
			events: []string{
				`{"type":"message_start","message":{"usage":{"input_tokens":25}}}`,
				`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
				`{"type":"message_delta","delta":{},"usage":{"output_tokens":12}}`,
			},
			wantPrompt:     25,
			wantCompletion: 12,
		},
		{
			name:     "gemini usage from usageMetadata",
			provider: "gemini",
			events: []string{
				`{"candidates":[{"content":{"parts":[{"text":"Hi"}]}}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":8}}`,
			},
			wantPrompt:     30,
			wantCompletion: 8,
		},
		{
			name:           "no usage data present",
			provider:       "openai",
			events:         []string{`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Hi"}}]}`},
			wantPrompt:     0,
			wantCompletion: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			accum := &TokenAccumulator{}
			for _, ev := range tt.events {
				accum.TryExtract(tt.provider, ev)
			}
			usage := accum.Total()
			assert.Equal(t, tt.wantPrompt, usage.PromptTokens, "prompt tokens")
			assert.Equal(t, tt.wantCompletion, usage.CompletionTokens, "completion tokens")
		})
	}
}

func TestStreamSSE(t *testing.T) {
	t.Parallel()

	t.Run("openai events forwarded as-is with usage extracted", func(t *testing.T) {
		t.Parallel()
		w := newTestResponseWriter()
		accum := &TokenAccumulator{}

		sseData := strings.Join([]string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")

		upstream := io.NopCloser(strings.NewReader(sseData))
		err := StreamSSE(context.Background(), w, upstream, "openai", accum)
		require.NoError(t, err)

		body := w.body.String()
		assert.Contains(t, body, `data: {"id":"chatcmpl-1"`)
		assert.Contains(t, body, "data: [DONE]")
		assert.Equal(t, "text/event-stream", w.header.Get("Content-Type"))
		assert.Equal(t, "no-cache", w.header.Get("Cache-Control"))
		assert.Equal(t, "no", w.header.Get("X-Accel-Buffering"))
		assert.Equal(t, http.StatusOK, w.statusCode)
		assert.Greater(t, w.flushed, 0, "should have flushed at least once")

		usage := accum.Total()
		assert.Equal(t, int64(10), usage.PromptTokens)
		assert.Equal(t, int64(5), usage.CompletionTokens)
	})

	t.Run("anthropic events translated to openai chunk format", func(t *testing.T) {
		t.Parallel()
		w := newTestResponseWriter()
		accum := &TokenAccumulator{}

		sseData := strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":25}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")

		upstream := io.NopCloser(strings.NewReader(sseData))
		err := StreamSSE(context.Background(), w, upstream, "anthropic", accum)
		require.NoError(t, err)

		body := w.body.String()
		// Verify content_block_delta was translated to OpenAI chunk format
		assert.Contains(t, body, `"content":"Hello"`)
		assert.Contains(t, body, `"object":"chat.completion.chunk"`)
		// Verify data: [DONE] is sent at the end
		assert.Contains(t, body, "data: [DONE]")

		usage := accum.Total()
		assert.Equal(t, int64(25), usage.PromptTokens)
		assert.Equal(t, int64(12), usage.CompletionTokens)
	})

	t.Run("gemini events translated to openai chunk format", func(t *testing.T) {
		t.Parallel()
		w := newTestResponseWriter()
		accum := &TokenAccumulator{}

		sseData := strings.Join([]string{
			`data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]},"index":0}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":8}}`,
			``,
		}, "\n")

		upstream := io.NopCloser(strings.NewReader(sseData))
		err := StreamSSE(context.Background(), w, upstream, "gemini", accum)
		require.NoError(t, err)

		body := w.body.String()
		assert.Contains(t, body, `"content":"Hi"`)
		assert.Contains(t, body, `"object":"chat.completion.chunk"`)
		assert.Contains(t, body, "data: [DONE]")

		usage := accum.Total()
		assert.Equal(t, int64(30), usage.PromptTokens)
		assert.Equal(t, int64(8), usage.CompletionTokens)
	})

	t.Run("client disconnect handled gracefully", func(t *testing.T) {
		t.Parallel()
		w := newTestResponseWriter()
		accum := &TokenAccumulator{}

		// Create a reader that blocks until context is cancelled
		ctx, cancel := context.WithCancel(context.Background())

		// Use a pipe so we can control when data arrives
		pr, pw := io.Pipe()

		// Write some data then cancel
		go func() {
			_, _ = pw.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
			cancel()
			_ = pw.Close()
		}()

		err := StreamSSE(ctx, w, pr, "openai", accum)
		// Should not return an error on client disconnect
		assert.NoError(t, err)
	})

	t.Run("empty stream still sends DONE", func(t *testing.T) {
		t.Parallel()
		w := newTestResponseWriter()
		accum := &TokenAccumulator{}

		upstream := io.NopCloser(strings.NewReader(""))
		err := StreamSSE(context.Background(), w, upstream, "openai", accum)
		require.NoError(t, err)

		body := w.body.String()
		assert.Contains(t, body, "data: [DONE]")
	})
}
