// Package provider defines the Provider interface contract for LLM provider adapters.
package provider

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
)

// Provider defines the contract for LLM provider adapters.
// Each provider (OpenAI, Anthropic, Gemini) implements this interface.
// Max 3 methods per CLAUDE.md interface rules.
type Provider interface {
	// Translate converts an OpenAI-format ChatRequest into the provider's native request format.
	Translate(req *ai.ChatRequest) (providerReq any, err error)

	// TranslateResponse converts the provider's native response bytes into OpenAI format.
	TranslateResponse(providerResp []byte, model string) (*ai.ChatResponse, error)

	// Call executes the translated request against the provider API.
	// For streaming requests, returns immediately; caller reads ProviderResult.Body.
	Call(ctx context.Context, providerReq any, streaming bool) (*ai.ProviderResult, error)
}

// ProviderConfig holds connection details for a single LLM provider.
type ProviderConfig struct {
	Name    string            `json:"name"`
	BaseURL string            `json:"base_url"`
	APIKey  string            `json:"-"` // never serialized
	Timeout time.Duration     `json:"timeout"`
	Models  map[string]string `json:"models"` // canonical name -> provider model name
}

// privateRanges contains CIDR blocks that must be blocked per security.md SSRF rules.
var privateRanges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"127.0.0.0/8",
	"::1/128",
	"fd00::/8",
	"fe80::/10",
}

var parsedPrivateNets []*net.IPNet

func init() {
	for _, cidr := range privateRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("parsing private CIDR %s: %v", cidr, err))
		}
		parsedPrivateNets = append(parsedPrivateNets, ipNet)
	}
}

// ValidateProviderURL checks that a provider base URL is well-formed and does not
// point to a private or loopback IP range. This prevents SSRF attacks per security.md.
func ValidateProviderURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("provider URL is empty")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("provider URL invalid: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("provider URL must use http or https scheme, got %q", parsed.Scheme)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("provider URL has no hostname")
	}

	// Block localhost by name.
	if hostname == "localhost" {
		return fmt.Errorf("provider URL must not point to localhost")
	}

	// Block cloud metadata hostname.
	if hostname == "metadata.google.internal" {
		return fmt.Errorf("provider URL must not point to cloud metadata endpoint")
	}

	// Resolve and check IP ranges.
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If the hostname doesn't resolve, it might be a DNS name that resolves later.
		// For IP literals, try parsing directly.
		ip := net.ParseIP(hostname)
		if ip == nil {
			// Can't resolve and not an IP literal -- allow it (DNS may resolve at call time).
			return nil
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		for _, ipNet := range parsedPrivateNets {
			if ipNet.Contains(ip) {
				return fmt.Errorf("provider URL resolves to private IP range %s", ipNet.String())
			}
		}
	}

	return nil
}
