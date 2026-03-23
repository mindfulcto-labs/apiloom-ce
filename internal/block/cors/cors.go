// Package cors implements a CORS block for the gateway proxy pipeline.
// It handles preflight OPTIONS requests and adds CORS response headers
// for allowed origins. Defaults to NO origins allowed (restrictive).
package cors

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/luraproject/lura/v2/proxy"
)

// CORSBlock implements the Block interface for Cross-Origin Resource Sharing.
type CORSBlock struct {
	config CORSConfig
	// allowedOriginsSet is a fast lookup set built from config.AllowedOrigins.
	allowedOriginsSet map[string]struct{}
	// allowedMethodsStr is a pre-joined string for the Access-Control-Allow-Methods header.
	allowedMethodsStr string
	// allowedHeadersStr is a pre-joined string for the Access-Control-Allow-Headers header.
	allowedHeadersStr string
	// maxAgeStr is the pre-formatted Access-Control-Max-Age value.
	maxAgeStr string
}

// NewCORSBlock creates a new CORSBlock with the given configuration.
// Returns an error if the configuration is invalid.
func NewCORSBlock(cfg CORSConfig) (*CORSBlock, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating CORS block: %w", err)
	}

	cfg.applyDefaults()

	// Build allowed origins lookup set, filtering out "*" unless AllowAllOrigins is explicitly set.
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" && !cfg.AllowAllOrigins {
			continue // Ignore wildcard without explicit opt-in
		}
		origins[strings.ToLower(o)] = struct{}{}
	}

	return &CORSBlock{
		config:            cfg,
		allowedOriginsSet: origins,
		allowedMethodsStr: strings.Join(cfg.AllowedMethods, ", "),
		allowedHeadersStr: strings.Join(cfg.AllowedHeaders, ", "),
		maxAgeStr:         strconv.Itoa(cfg.MaxAge),
	}, nil
}

// Name returns the block identifier.
func (b *CORSBlock) Name() string {
	return "cors"
}

// Middleware returns a Lura proxy.Middleware that handles CORS.
// Preflight OPTIONS requests are short-circuited (next proxy is NOT called).
// Normal requests with a valid Origin get CORS response headers added.
func (b *CORSBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			origin := headerGet(req.Headers, "Origin")

			// No Origin header = not a CORS request; pass through
			if origin == "" {
				return nextProxy(ctx, req)
			}

			allowed := b.isOriginAllowed(origin)

			// Preflight: OPTIONS with Access-Control-Request-Method
			if req.Method == http.MethodOptions && headerGet(req.Headers, "Access-Control-Request-Method") != "" {
				return b.handlePreflight(origin, req, allowed)
			}

			// Normal request: call next, then add CORS headers if allowed
			resp, err := nextProxy(ctx, req)
			if err != nil {
				return resp, err
			}

			if allowed {
				b.setCORSHeaders(resp, origin)
			}

			return resp, nil
		}
	}
}

// handlePreflight builds a preflight response. It short-circuits the proxy chain.
func (b *CORSBlock) handlePreflight(origin string, req *proxy.Request, allowed bool) (*proxy.Response, error) {
	headers := make(map[string][]string)

	if allowed {
		if b.config.AllowAllOrigins {
			headerSet(headers, "Access-Control-Allow-Origin", "*")
		} else {
			headerSet(headers, "Access-Control-Allow-Origin", origin)
		}

		headerSet(headers, "Access-Control-Allow-Methods", b.allowedMethodsStr)
		headerSet(headers, "Access-Control-Allow-Headers", b.allowedHeadersStr)
		headerSet(headers, "Access-Control-Max-Age", b.maxAgeStr)

		if b.config.AllowCredentials {
			headerSet(headers, "Access-Control-Allow-Credentials", "true")
		}

		headerSet(headers, "Vary", "Origin")
	}

	return &proxy.Response{
		IsComplete: true,
		Metadata: proxy.Metadata{
			StatusCode: http.StatusOK,
			Headers:    headers,
		},
	}, nil
}

// setCORSHeaders adds CORS response headers to a normal (non-preflight) response.
func (b *CORSBlock) setCORSHeaders(resp *proxy.Response, origin string) {
	if resp.Metadata.Headers == nil {
		resp.Metadata.Headers = make(map[string][]string)
	}

	if b.config.AllowAllOrigins {
		headerSet(resp.Metadata.Headers, "Access-Control-Allow-Origin", "*")
	} else {
		headerSet(resp.Metadata.Headers, "Access-Control-Allow-Origin", origin)
	}

	if b.config.AllowCredentials {
		headerSet(resp.Metadata.Headers, "Access-Control-Allow-Credentials", "true")
	}

	headerSet(resp.Metadata.Headers, "Vary", "Origin")
}

// isOriginAllowed checks if the given origin is permitted.
func (b *CORSBlock) isOriginAllowed(origin string) bool {
	if b.config.AllowAllOrigins {
		_, has := b.allowedOriginsSet[strings.ToLower("*")]
		return has
	}
	if len(b.allowedOriginsSet) == 0 {
		return false
	}
	_, ok := b.allowedOriginsSet[strings.ToLower(origin)]
	return ok
}

// headerGet retrieves the first value for a header key from map[string][]string.
func headerGet(headers map[string][]string, key string) string {
	return http.Header(headers).Get(key)
}

// headerSet sets a header key to a single value in map[string][]string.
func headerSet(headers map[string][]string, key, value string) {
	http.Header(headers).Set(key, value)
}
