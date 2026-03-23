package wasm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const fetchTimeout = 10 * time.Second

// HostFuncs holds the configuration for host functions exposed to WASM modules.
// Currently provides: http_fetch — guarded by capability allowlist.
type HostFuncs struct {
	caps   CapabilitySet
	client *http.Client
}

// NewHostFuncs constructs a HostFuncs with the given capability set and HTTP client.
// If client is nil, http.DefaultClient is used.
func NewHostFuncs(caps CapabilitySet, client *http.Client) *HostFuncs {
	if client == nil {
		client = http.DefaultClient
	}
	return &HostFuncs{
		caps:   caps,
		client: client,
	}
}

// FetchURL performs a capability-checked HTTP GET to the given URL.
// It enforces a 10-second timeout and strips Authorization headers from the response.
// Returns the response body bytes, HTTP status code, and any error.
//
// This function is also called by the WASM http_fetch host function.
// Exposed as a package-level function for testability.
func FetchURL(ctx context.Context, caps CapabilitySet, client *http.Client, rawURL string) ([]byte, int, error) {
	if client == nil {
		client = http.DefaultClient
	}

	// Parse the URL to extract the host for capability check.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, 0, fmt.Errorf("wasm: invalid url %q: %w", rawURL, err)
	}

	host := parsed.Hostname()
	if !caps.AllowsHost(host) {
		return nil, 0, fmt.Errorf("wasm: host %q not in capability set", host)
	}

	// Apply timeout on the fetch.
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("wasm: creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("wasm: fetching %q: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Drain and collect the response body (limit to 1MB).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("wasm: reading response body: %w", err)
	}

	// Strip sensitive headers from the response before returning to WASM module.
	resp.Header.Del("Authorization")
	resp.Header.Del("Set-Cookie")

	return body, resp.StatusCode, nil
}
