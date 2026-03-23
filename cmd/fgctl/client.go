package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ControlPlaneClient is a simple HTTP client for the control plane REST API.
type ControlPlaneClient struct {
	baseURL string
	orgID   string
	http    *http.Client
}

// NewControlPlaneClient creates a client targeting the given base URL and org.
func NewControlPlaneClient(baseURL, orgID string) *ControlPlaneClient {
	return &ControlPlaneClient{
		baseURL: baseURL,
		orgID:   orgID,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *ControlPlaneClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", c.orgID)

	return c.http.Do(req)
}

func (c *ControlPlaneClient) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
}
