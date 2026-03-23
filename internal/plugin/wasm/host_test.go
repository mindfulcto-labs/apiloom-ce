package wasm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/plugin/wasm"
)

func TestHostFuncs_NotNil(t *testing.T) {
	t.Parallel()
	caps := wasm.NewCapabilitySet([]string{"api.example.com"})
	hf := wasm.NewHostFuncs(caps, nil)
	require.NotNil(t, hf)
}

func TestHostFuncs_WithCustomHTTPClient(t *testing.T) {
	t.Parallel()
	caps := wasm.NewCapabilitySet([]string{"api.example.com"})
	client := &http.Client{}
	hf := wasm.NewHostFuncs(caps, client)
	require.NotNil(t, hf)
}

// TestHostFetch_CapabilityCheck verifies that capability checks work correctly
// for known blocked hosts (private IPs, metadata endpoints).
func TestHostFetch_CapabilityCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hosts   []string
		check   string
		allowed bool
	}{
		{
			name:    "declared public host is allowed",
			hosts:   []string{"api.example.com"},
			check:   "api.example.com",
			allowed: true,
		},
		{
			name:    "metadata endpoint 169.254.169.254 always blocked",
			hosts:   []string{"169.254.169.254"},
			check:   "169.254.169.254",
			allowed: false,
		},
		{
			name:    "private IP 10.0.0.1 always blocked",
			hosts:   []string{"10.0.0.1"},
			check:   "10.0.0.1",
			allowed: false,
		},
		{
			name:    "loopback 127.0.0.1 always blocked",
			hosts:   []string{"127.0.0.1"},
			check:   "127.0.0.1",
			allowed: false,
		},
		{
			name:    "undeclared host is blocked",
			hosts:   []string{"api.example.com"},
			check:   "other.example.com",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			caps := wasm.NewCapabilitySet(tt.hosts)
			assert.Equal(t, tt.allowed, caps.AllowsHost(tt.check))
		})
	}
}

// TestFetchURL_BlockedHosts verifies FetchURL returns errors for blocked hosts.
func TestFetchURL_BlockedHosts(t *testing.T) {
	t.Parallel()

	caps := wasm.NewCapabilitySet([]string{"api.example.com"})

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "disallowed host returns error",
			url:  "http://evil.example.com/data",
		},
		{
			name: "metadata endpoint returns error",
			url:  "http://169.254.169.254/latest/metadata",
		},
		{
			name: "private IP returns error",
			url:  "http://10.0.0.1/secret",
		},
		{
			name: "loopback returns error",
			url:  "http://127.0.0.1/secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			_, _, err := wasm.FetchURL(ctx, caps, nil, tt.url)
			require.Error(t, err, "expected error for blocked URL: %s", tt.url)
		})
	}
}

// TestFetchURL_AllowedHost verifies FetchURL succeeds for a declared non-private host.
// Uses a test HTTP server with a domain alias to avoid loopback blocking.
func TestFetchURL_AllowedHost(t *testing.T) {
	t.Parallel()

	// We need a hostname that resolves to localhost but is NOT in the blocked private range.
	// Since we can't easily do that in a unit test without DNS setup, we verify the
	// capability check passes for public-looking hostnames using a mock round tripper.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure Authorization header is NOT forwarded (it's a request header we don't set,
		// but verify the response stripping works — see below)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`hello`)) //nolint:errcheck
	}))
	defer ts.Close()

	// httptest.NewServer binds to 127.0.0.1 (loopback), which is blocked by SSRF rules.
	// This is correct security behavior. We test the allowed-host path using a custom
	// transport that intercepts requests to a fictional external hostname.
	targetHost := "api.testonly.example"
	caps := wasm.NewCapabilitySet([]string{targetHost})

	// Custom transport: intercept requests to api.testonly.example and proxy to test server.
	transport := &mockTransport{delegate: ts.Client().Transport, targetURL: ts.URL}
	client := &http.Client{Transport: transport}

	ctx := context.Background()
	body, statusCode, err := wasm.FetchURL(ctx, caps, client, "http://"+targetHost+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "hello", string(body))
}

// mockTransport intercepts requests to a fictional hostname and routes to a real test server.
type mockTransport struct {
	delegate  http.RoundTripper
	targetURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the host in the request URL with the test server URL.
	newURL := strings.Replace(req.URL.String(), "http://"+req.Host, m.targetURL, 1)
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	if m.delegate == nil {
		return http.DefaultTransport.RoundTrip(newReq)
	}
	return m.delegate.RoundTrip(newReq)
}
