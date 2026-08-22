// Package openai implements the Provider interface for the OpenAI API.
// Since OpenAI is the canonical format, translation is minimal (passthrough).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
	"github.com/mindfulcto-labs/apiloom-ce/internal/ai/provider"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

const (
	defaultTimeout = 120 * time.Second
	maxResponseBody = 10 << 20 // 10 MB
)

// Adapter implements provider.Provider for the OpenAI API.
type Adapter struct {
	client  *http.Client
	config  provider.ProviderConfig
}

// New creates a new OpenAI adapter after validating the base URL against SSRF rules.
func New(cfg provider.ProviderConfig) (*Adapter, error) {
	if err := provider.ValidateProviderURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("openai provider URL: %w", err)
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

// Translate marshals a ChatRequest as-is to JSON since OpenAI is the canonical format.
// For streaming requests, it ensures stream_options.include_usage is set.
func (a *Adapter) Translate(req *ai.ChatRequest) (any, error) {
	if req.Stream && req.StreamOptions == nil {
		req.StreamOptions = &ai.StreamOptions{IncludeUsage: true}
	}

	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling openai request: %w", err)
	}
	return b, nil
}

// TranslateResponse unmarshals provider response bytes directly to ChatResponse.
func (a *Adapter) TranslateResponse(providerResp []byte, model string) (*ai.ChatResponse, error) {
	var resp ai.ChatResponse
	if err := json.Unmarshal(providerResp, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling openai response: %w", err)
	}
	return &resp, nil
}

// Call executes the request against the OpenAI API.
// For streaming, it returns the body as-is for the caller to consume as SSE.
func (a *Adapter) Call(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error) {
	body, ok := providerReq.([]byte)
	if !ok {
		return nil, fmt.Errorf("openai: expected []byte request, got %T", providerReq)
	}

	url := a.config.BaseURL + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating openai request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling openai: %w", err)
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

	// Non-streaming: wrap body with limit reader.
	limited := io.LimitReader(resp.Body, maxResponseBody)
	respBody, err := io.ReadAll(limited)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading openai response: %w", err)
	}

	return &ai.ProviderResult{
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		StatusCode: resp.StatusCode,
		Streaming:  false,
	}, nil
}

// handleError reads the error body, extracts the message, and returns a sanitized ProblemError.
// Raw provider body is never forwarded per security.md.
func (a *Adapter) handleError(resp *http.Response) error {
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBody)
	body, _ := io.ReadAll(limited)

	msg := extractErrorMessage(body)
	if msg == "" {
		msg = fmt.Sprintf("openai returned HTTP %d", resp.StatusCode)
	}

	return domain.NewProblemError(
		resp.StatusCode,
		"provider-error",
		fmt.Sprintf("openai: %s", msg),
	)
}

// extractErrorMessage tries to pull the error.message from an OpenAI error JSON.
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
