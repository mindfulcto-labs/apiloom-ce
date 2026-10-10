// Package aiproxy implements the AI proxy HTTP handler block for FlexGateway.
// It orchestrates request routing, budget enforcement, SSE streaming, and
// fallback chains for the /v1/chat/completions endpoint.
package aiproxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
)

// TokenAccumulator accumulates token usage from intercepted SSE events.
type TokenAccumulator struct {
	PromptTokens     int64
	CompletionTokens int64
}

// TryExtract parses JSON event data and extracts usage based on provider format.
func (a *TokenAccumulator) TryExtract(provider string, jsonData string) {
	switch provider {
	case "openai":
		a.extractOpenAI(jsonData)
	case "anthropic":
		a.extractAnthropic(jsonData)
	case "gemini":
		a.extractGemini(jsonData)
	}
}

func (a *TokenAccumulator) extractOpenAI(data string) {
	var chunk struct {
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(data), &chunk) == nil && chunk.Usage != nil {
		a.PromptTokens += chunk.Usage.PromptTokens
		a.CompletionTokens += chunk.Usage.CompletionTokens
	}
}

func (a *TokenAccumulator) extractAnthropic(data string) {
	var event struct {
		Type    string `json:"type"`
		Message *struct {
			Usage struct {
				InputTokens int64 `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage *struct {
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(data), &event) != nil {
		return
	}
	if event.Type == "message_start" && event.Message != nil {
		a.PromptTokens += event.Message.Usage.InputTokens
	}
	if event.Type == "message_delta" && event.Usage != nil {
		a.CompletionTokens += event.Usage.OutputTokens
	}
}

func (a *TokenAccumulator) extractGemini(data string) {
	var resp struct {
		UsageMetadata *struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if json.Unmarshal([]byte(data), &resp) == nil && resp.UsageMetadata != nil {
		a.PromptTokens = resp.UsageMetadata.PromptTokenCount
		a.CompletionTokens = resp.UsageMetadata.CandidatesTokenCount
	}
}

// Total returns the accumulated usage.
func (a *TokenAccumulator) Total() ai.Usage {
	return ai.Usage{
		PromptTokens:     a.PromptTokens,
		CompletionTokens: a.CompletionTokens,
		TotalTokens:      a.PromptTokens + a.CompletionTokens,
	}
}

// StreamSSE forwards SSE events from upstream to the client, translating
// provider-specific formats to OpenAI chunk format as needed.
// It disables WriteTimeout per-request via http.NewResponseController,
// sets SSE headers, and flushes after every line.
func StreamSSE(ctx context.Context, w http.ResponseWriter, upstream io.Reader, provider string, accum *TokenAccumulator) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not implement http.Flusher")
	}

	// Disable WriteTimeout for streaming via ResponseController.
	// This is critical per CLAUDE.md pitfall #5: SSE streaming must not be
	// killed by the server's default WriteTimeout.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// Set SSE headers before writing any data.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(upstream)
	sentDone := false

	for scanner.Scan() {
		// Check for client disconnect.
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()

		switch provider {
		case "openai":
			sentDone = handleOpenAILine(w, flusher, line, accum, sentDone)
		case "anthropic":
			sentDone = handleAnthropicLine(w, flusher, line, accum, sentDone)
		case "gemini":
			handleGeminiLine(w, flusher, line, accum)
		}
	}

	// Always terminate with data: [DONE] if not already sent.
	if !sentDone {
		writeLine(w, flusher, "data: [DONE]")
		writeLine(w, flusher, "")
	}

	return nil
}

// handleOpenAILine forwards OpenAI SSE lines as-is, intercepting usage.
func handleOpenAILine(w http.ResponseWriter, f http.Flusher, line string, accum *TokenAccumulator, sentDone bool) bool {
	if line == "" {
		writeLine(w, f, "")
		return sentDone
	}

	if strings.HasPrefix(line, "data: ") {
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			writeLine(w, f, line)
			writeLine(w, f, "")
			return true
		}
		accum.TryExtract("openai", payload)
		writeLine(w, f, line)
		return sentDone
	}

	// Forward any other lines (event:, id:, etc.) as-is.
	writeLine(w, f, line)
	return sentDone
}

// handleAnthropicLine translates Anthropic SSE events to OpenAI chunk format.
func handleAnthropicLine(w http.ResponseWriter, f http.Flusher, line string, accum *TokenAccumulator, sentDone bool) bool {
	if line == "" || strings.HasPrefix(line, "event:") {
		// Skip event: lines and blank lines (we generate our own framing).
		return sentDone
	}

	if !strings.HasPrefix(line, "data: ") {
		return sentDone
	}

	payload := strings.TrimPrefix(line, "data: ")
	accum.TryExtract("anthropic", payload)

	// Parse the Anthropic event to determine translation.
	var event struct {
		Type  string `json:"type"`
		Delta *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Index int `json:"index"`
	}
	if json.Unmarshal([]byte(payload), &event) != nil {
		return sentDone
	}

	switch event.Type {
	case "content_block_delta":
		if event.Delta != nil && event.Delta.Type == "text_delta" {
			chunk := ai.StreamChunk{
				ID:      "chatcmpl-anthropic",
				Object:  "chat.completion.chunk",
				Created: 0,
				Choices: []ai.StreamChoice{
					{
						Index: event.Index,
						Delta: ai.ChatMessage{Content: event.Delta.Text},
					},
				},
			}
			data, err := json.Marshal(chunk)
			if err == nil {
				writeLine(w, f, "data: "+string(data))
				writeLine(w, f, "")
			}
		}
	case "message_stop":
		writeLine(w, f, "data: [DONE]")
		writeLine(w, f, "")
		return true
	}

	return sentDone
}

// handleGeminiLine translates Gemini SSE events to OpenAI chunk format.
func handleGeminiLine(w http.ResponseWriter, f http.Flusher, line string, accum *TokenAccumulator) {
	if line == "" {
		return
	}

	if !strings.HasPrefix(line, "data: ") {
		return
	}

	payload := strings.TrimPrefix(line, "data: ")
	if payload == "[DONE]" {
		writeLine(w, f, line)
		writeLine(w, f, "")
		return
	}

	accum.TryExtract("gemini", payload)

	// Parse Gemini response.
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			Index int `json:"index"`
		} `json:"candidates"`
	}
	if json.Unmarshal([]byte(payload), &resp) != nil {
		return
	}

	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			chunk := ai.StreamChunk{
				ID:      "chatcmpl-gemini",
				Object:  "chat.completion.chunk",
				Created: 0,
				Choices: []ai.StreamChoice{
					{
						Index: candidate.Index,
						Delta: ai.ChatMessage{Content: part.Text},
					},
				},
			}
			data, err := json.Marshal(chunk)
			if err == nil {
				writeLine(w, f, "data: "+string(data))
				writeLine(w, f, "")
			}
		}
	}
}

// writeLine writes a line to the response writer and flushes.
func writeLine(w http.ResponseWriter, f http.Flusher, line string) {
	_, _ = fmt.Fprintf(w, "%s\n", line)
	f.Flush()
}
