// Package gemini implements the Provider interface for Google's Gemini API.
// It translates between OpenAI-compatible format and Gemini's native format,
// handling role mapping (assistant->model), system instruction extraction,
// and streaming endpoint selection (generateContent vs streamGenerateContent).
package gemini

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
	defaultTimeout  = 120 * time.Second
	maxResponseBody = 10 << 20 // 10 MB
)

// Adapter implements provider.Provider for the Gemini API.
type Adapter struct {
	client   *http.Client
	config   provider.ProviderConfig
	authMode string // "apikey" or "bearer"
}

// New creates a new Gemini adapter after validating the base URL against SSRF rules.
// AuthMode defaults to "apikey" (uses ?key= query param). Set Models["auth_mode"]="bearer"
// for service account / OAuth Bearer token authentication.
func New(cfg provider.ProviderConfig) (*Adapter, error) {
	if err := provider.ValidateProviderURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("gemini provider URL: %w", err)
	}

	authMode := "apikey"
	if cfg.Models != nil {
		if mode, ok := cfg.Models["auth_mode"]; ok {
			authMode = mode
		}
	}

	return newAdapter(cfg, authMode), nil
}

// newAdapter builds the adapter without URL validation (used by tests via testAdapter).
func newAdapter(cfg provider.ProviderConfig, authMode string) *Adapter {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Adapter{
		client:   &http.Client{Timeout: timeout},
		config:   cfg,
		authMode: authMode,
	}
}

// geminiCallPayload carries the translated request body along with metadata
// needed by Call to construct the correct URL (model name, streaming flag).
type geminiCallPayload struct {
	model     string
	body      any
	streaming bool
}

// Gemini request types.
type geminiRequest struct {
	Contents          []geminiContent     `json:"contents"`
	SystemInstruction *geminiContent      `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig   `json:"generationConfig,omitempty"`
	Tools             []geminiToolSet     `json:"tools,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string               `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall   `json:"functionCall,omitempty"`
}

type geminiFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type generationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
}

type geminiToolSet struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Translate converts an OpenAI ChatRequest into Gemini's native format.
func (a *Adapter) Translate(req *ai.ChatRequest) (any, error) {
	gr := geminiRequest{}

	// Extract system messages to systemInstruction.
	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if s, ok := msg.Content.(string); ok {
				systemParts = append(systemParts, s)
			}
			continue
		}

		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		content := ""
		if s, ok := msg.Content.(string); ok {
			content = s
		}

		gr.Contents = append(gr.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: content}},
		})
	}

	if len(systemParts) > 0 {
		gr.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: strings.Join(systemParts, "\n\n")}},
		}
	}

	// Map generation config.
	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil {
		gc := &generationConfig{
			Temperature: req.Temperature,
			TopP:        req.TopP,
		}
		if req.MaxTokens != nil {
			gc.MaxOutputTokens = req.MaxTokens
		}
		gr.GenerationConfig = gc
	}

	// Map tools to functionDeclarations.
	if len(req.Tools) > 0 {
		var decls []geminiFunctionDecl
		for _, tool := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			})
		}
		gr.Tools = []geminiToolSet{{FunctionDeclarations: decls}}
	}

	return geminiCallPayload{
		model:     req.Model,
		body:      gr,
		streaming: req.Stream,
	}, nil
}

// Gemini response types.
type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	TotalTokenCount      int64 `json:"totalTokenCount"`
}

// TranslateResponse converts Gemini's response to OpenAI-compatible format.
func (a *Adapter) TranslateResponse(providerResp []byte, model string) (*ai.ChatResponse, error) {
	var gr geminiResponse
	if err := json.Unmarshal(providerResp, &gr); err != nil {
		return nil, fmt.Errorf("unmarshaling gemini response: %w", err)
	}

	resp := &ai.ChatResponse{
		Object: "chat.completion",
		Model:  model,
	}

	if len(gr.Candidates) > 0 {
		cand := gr.Candidates[0]

		msg := ai.ChatMessage{
			Role: "assistant",
		}

		// Extract text from parts.
		var textParts []string
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				msg.ToolCalls = append(msg.ToolCalls, ai.ToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: ai.ToolCallFunction{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				})
			}
		}

		if len(textParts) > 0 {
			msg.Content = strings.Join(textParts, "")
		} else {
			msg.Content = ""
		}

		resp.Choices = []ai.Choice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: mapFinishReason(cand.FinishReason),
			},
		}
	}

	if gr.UsageMetadata != nil {
		resp.Usage = ai.Usage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.PromptTokenCount + gr.UsageMetadata.CandidatesTokenCount,
		}
	}

	return resp, nil
}

// mapFinishReason maps Gemini finish reasons to OpenAI format.
func mapFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return reason
	}
}

// Call executes the request against the Gemini API.
// Non-streaming: POST /v1beta/models/{model}:generateContent
// Streaming: POST /v1beta/models/{model}:streamGenerateContent?alt=sse
func (a *Adapter) Call(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error) {
	payload, ok := providerReq.(geminiCallPayload)
	if !ok {
		return nil, fmt.Errorf("gemini: expected geminiCallPayload, got %T", providerReq)
	}

	body, err := json.Marshal(payload.body)
	if err != nil {
		return nil, fmt.Errorf("marshaling gemini request body: %w", err)
	}

	// Construct URL based on streaming flag.
	var reqURL string
	if streaming {
		reqURL = fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent", a.config.BaseURL, payload.model)
	} else {
		reqURL = fmt.Sprintf("%s/v1beta/models/%s:generateContent", a.config.BaseURL, payload.model)
	}

	// Add auth to URL or headers.
	switch a.authMode {
	case "apikey":
		if streaming {
			reqURL += "?alt=sse&key=" + a.config.APIKey
		} else {
			reqURL += "?key=" + a.config.APIKey
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating gemini request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if a.authMode == "bearer" {
		httpReq.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling gemini: %w", err)
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
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading gemini response: %w", err)
	}

	return &ai.ProviderResult{
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		StatusCode: resp.StatusCode,
		Streaming:  false,
	}, nil
}

// handleError reads the error body, extracts the message, and returns a sanitized ProblemError.
func (a *Adapter) handleError(resp *http.Response) error {
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBody)
	body, _ := io.ReadAll(limited)

	msg := extractErrorMessage(body)
	if msg == "" {
		msg = fmt.Sprintf("gemini returned HTTP %d", resp.StatusCode)
	}

	return domain.NewProblemError(
		resp.StatusCode,
		"provider-error",
		fmt.Sprintf("gemini: %s", msg),
	)
}

// extractErrorMessage tries to pull the error.message from a Gemini error JSON.
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
