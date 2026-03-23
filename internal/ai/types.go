// Package ai provides domain types for the AI gateway: OpenAI-compatible
// request/response structures, model definitions, and provider result types.
package ai

import "io"

// ChatRequest represents an OpenAI-compatible chat completion request.
// JSON tags match the OpenAI API snake_case convention exactly.
type ChatRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Temperature      *float64        `json:"temperature,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       any             `json:"tool_choice,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	User             string          `json:"user,omitempty"`
	StreamOptions    *StreamOptions  `json:"stream_options,omitempty"`

	// RequiredCapabilities is a gateway-only field (not sent to providers)
	// specifying model capabilities needed for this request.
	RequiredCapabilities []string `json:"-"`
}

// ChatResponse represents an OpenAI-compatible chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Choice represents a single completion choice in a response.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage tracks token consumption for a request.
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// Tool represents a tool definition in the request.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function tool.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool invocation in an assistant message.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StreamChunk represents a single SSE chunk in a streaming response.
type StreamChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// StreamChoice is a choice within a streaming chunk, using delta instead of message.
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamOptions controls streaming behavior.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Model represents an AI model with pricing and capability metadata.
type Model struct {
	Provider            string   `json:"provider"`
	ModelName           string   `json:"model_name"`
	DisplayName         string   `json:"display_name"`
	InputPricePerToken  float64  `json:"input_price_per_token"`
	OutputPricePerToken float64  `json:"output_price_per_token"`
	Capabilities        []string `json:"capabilities"`
	IsActive            bool     `json:"is_active"`
	Weight              int      `json:"weight"`
}

// ProviderResult encapsulates the raw response from an upstream LLM provider.
type ProviderResult struct {
	Body       io.ReadCloser
	StatusCode int
	Streaming  bool
}
