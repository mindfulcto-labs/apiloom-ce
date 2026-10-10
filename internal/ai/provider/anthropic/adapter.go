// Package anthropic implements the Provider interface for the Anthropic Messages API.
// It translates between OpenAI-compatible format and Anthropic's native format,
// handling system message extraction, role mapping, and response translation.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

const (
	defaultTimeout    = 120 * time.Second
	maxResponseBody   = 10 << 20 // 10 MB
	defaultMaxTokens  = 4096
	anthropicVersion  = "2023-06-01"
)

// Adapter implements provider.Provider for the Anthropic Messages API.
type Adapter struct {
	client *http.Client
	config provider.ProviderConfig
}

// New creates a new Anthropic adapter after validating the base URL against SSRF rules.
func New(cfg provider.ProviderConfig) (*Adapter, error) {
	if err := provider.ValidateProviderURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("anthropic provider URL: %w", err)
	}
	return newAdapter(cfg), nil
}

// newAdapter builds the adapter without URL validation (used by tests via newForTest).
func newAdapter(cfg provider.ProviderConfig) *Adapter {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Adapter{
		client: &http.Client{Timeout: timeout},
		config: cfg,
	}
}

// anthropicRequest is the native Anthropic Messages API request format.
type anthropicRequest struct {
	Model      string            `json:"model"`
	Messages   []anthropicMsg    `json:"messages"`
	System     string            `json:"system,omitempty"`
	MaxTokens  int               `json:"max_tokens"`
	Temp       *float64          `json:"temperature,omitempty"`
	TopP       *float64          `json:"top_p,omitempty"`
	Stream     bool              `json:"stream,omitempty"`
	Tools      []anthropicTool   `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice `json:"tool_choice,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// Translate converts an OpenAI ChatRequest into Anthropic's native format.
// It extracts system messages to a top-level field and sets default max_tokens.
func (a *Adapter) Translate(req *ai.ChatRequest) (any, error) {
	ar := anthropicRequest{
		Model:  req.Model,
		Temp:   req.Temperature,
		TopP:   req.TopP,
		Stream: req.Stream,
	}

	// Extract system messages.
	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if s, ok := msg.Content.(string); ok {
				systemParts = append(systemParts, s)
			}
			continue
		}
		ar.Messages = append(ar.Messages, anthropicMsg{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	if len(systemParts) > 0 {
		ar.System = strings.Join(systemParts, "\n\n")
	}

	// Set max_tokens (required by Anthropic).
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = defaultMaxTokens
	}

	// Map tools.
	for _, tool := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}

	// Map tool_choice.
	if req.ToolChoice != nil {
		ar.ToolChoice = mapToolChoice(req.ToolChoice)
	}

	b, err := json.Marshal(ar)
	if err != nil {
		return nil, fmt.Errorf("marshaling anthropic request: %w", err)
	}
	return b, nil
}

// mapToolChoice converts OpenAI tool_choice to Anthropic format.
func mapToolChoice(tc any) *anthropicToolChoice {
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return &anthropicToolChoice{Type: "auto"}
		case "none":
			return nil
		case "required":
			return &anthropicToolChoice{Type: "any"}
		default:
			return &anthropicToolChoice{Type: "auto"}
		}
	case map[string]any:
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				return &anthropicToolChoice{Type: "tool", Name: name}
			}
		}
	}
	return nil
}

// Anthropic response types.
type anthropicResponse struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Role       string              `json:"role"`
	Content    []anthropicContent  `json:"content"`
	Model      string              `json:"model"`
	StopReason string              `json:"stop_reason"`
	Usage      anthropicUsage      `json:"usage"`
}

type anthropicContent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// TranslateResponse converts Anthropic's response to OpenAI-compatible format.
func (a *Adapter) TranslateResponse(providerResp []byte, model string) (*ai.ChatResponse, error) {
	var ar anthropicResponse
	if err := json.Unmarshal(providerResp, &ar); err != nil {
		return nil, fmt.Errorf("unmarshaling anthropic response: %w", err)
	}

	msg := ai.ChatMessage{
		Role: "assistant",
	}

	// Extract text content and tool calls.
	var textParts []string
	for _, c := range ar.Content {
		switch c.Type {
		case "text":
			textParts = append(textParts, c.Text)
		case "tool_use":
			args, _ := json.Marshal(c.Input)
			msg.ToolCalls = append(msg.ToolCalls, ai.ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: ai.ToolCallFunction{
					Name:      c.Name,
					Arguments: string(args),
				},
			})
		}
	}
	if len(textParts) > 0 {
		msg.Content = strings.Join(textParts, "")
	}

	finishReason := mapStopReason(ar.StopReason)

	return &ai.ChatResponse{
		ID:      ar.ID,
		Object:  "chat.completion",
		Model:   model,
		Choices: []ai.Choice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: ai.Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		},
	}, nil
}

// mapStopReason maps Anthropic stop reasons to OpenAI finish reasons.
func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}

// Call executes the request against the Anthropic Messages API.
func (a *Adapter) Call(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error) {
	body, ok := providerReq.([]byte)
	if !ok {
		return nil, fmt.Errorf("anthropic: expected []byte request, got %T", providerReq)
	}

	url := a.config.BaseURL + "/v1/messages"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating anthropic request: %w", err)
	}

	httpReq.Header.Set("x-api-key", a.config.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling anthropic: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, a.handleError(resp)
	}

	if streaming {
		return &ai.ProviderResult{
			Body:       resp.Body,
			StatusCode: resp.StatusCode,
			Streaming:  true,
		}, nil
	}

	limited := io.LimitReader(resp.Body, maxResponseBody)
	respBody, err := io.ReadAll(limited)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading anthropic response: %w", err)
	}

	return &ai.ProviderResult{
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		StatusCode: resp.StatusCode,
		Streaming:  false,
	}, nil
}

// handleError reads the error body, extracts the message, and returns a sanitized ProblemError.
func (a *Adapter) handleError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxResponseBody)
	body, _ := io.ReadAll(limited)

	msg := extractErrorMessage(body)
	if msg == "" {
		msg = fmt.Sprintf("anthropic returned HTTP %d", resp.StatusCode)
	}

	return domain.NewProblemError(
		resp.StatusCode,
		"provider-error",
		fmt.Sprintf("anthropic: %s", msg),
	)
}

// extractErrorMessage extracts the error message from Anthropic error response JSON.
func extractErrorMessage(body []byte) string {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return ""
}
