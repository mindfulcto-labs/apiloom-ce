package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	luraconfig "github.com/luraproject/lura/v2/config"
	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// ConfigProvider is satisfied by config.Loader or any type with an atomic Get().
type ConfigProvider interface {
	Get() *domain.GatewayConfig
}

// ServerConfig holds configuration for the gateway HTTP server.
type ServerConfig struct {
	ListenAddr   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Server is the gateway HTTP server. It reads config from a ConfigProvider,
// builds a Lura-based routing table, and proxies requests to upstreams.
type Server struct {
	httpServer *http.Server
	config     ConfigProvider
	chain      *BlockChain
	logger     *slog.Logger
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithLogger sets the server's logger.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// NewServer creates a new gateway Server.
func NewServer(cfg ServerConfig, configLoader ConfigProvider, chain *BlockChain, opts ...ServerOption) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 120 * time.Second
	}

	s := &Server{
		config: configLoader,
		chain:  chain,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.httpServer = &http.Server{
		Addr:         cfg.ListenAddr,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return s
}

// Start builds the routing table from the current config and starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	gc := s.config.Get()
	if gc == nil {
		return fmt.Errorf("starting server: config not loaded")
	}

	handler, err := BuildHandler(s.config, s.chain)
	if err != nil {
		return fmt.Errorf("building handler: %w", err)
	}
	s.httpServer.Handler = handler

	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.httpServer.Addr, err)
	}

	s.logger.Info("gateway server starting", "addr", ln.Addr().String())

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("gateway server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// RouteConfigDetail is the parsed JSON structure inside RouteConfig.Config.
type RouteConfigDetail struct {
	Path        string   `json:"path"`
	Method      string   `json:"method"`
	BackendURLs []string `json:"backend_urls"`
	Timeout     string   `json:"timeout,omitempty"`
	Headers     []string `json:"headers_to_pass,omitempty"`
}

// BuildHandlerOption configures optional features for BuildHandler.
type BuildHandlerOption func(*buildHandlerOpts)

type buildHandlerOpts struct {
	aiHandler http.Handler
}

// WithAIHandler registers the AI proxy handler at /v1/chat/completions.
// The block chain middleware applies to AI routes automatically via chi's
// middleware stack, ensuring auth and rate limiting work on AI requests.
func WithAIHandler(h http.Handler) BuildHandlerOption {
	return func(o *buildHandlerOpts) {
		o.aiHandler = h
	}
}

// BuildHandler constructs an http.Handler from the gateway config and block chain.
// This is exposed for testing — it builds the chi router with proxy handlers
// without starting a listener.
func BuildHandler(provider ConfigProvider, chain *BlockChain, opts ...BuildHandlerOption) (http.Handler, error) {
	var o buildHandlerOpts
	for _, opt := range opts {
		opt(&o)
	}

	gc := provider.Get()
	if gc == nil {
		return notFoundHandler(), nil
	}

	sc, err := buildServiceConfig(gc, ":0")
	if err != nil {
		return nil, fmt.Errorf("building service config: %w", err)
	}

	r := chi.NewRouter()

	chainMW := chain.Middleware()

	for _, ep := range sc.Endpoints {
		handler := buildEndpointHandler(ep, chainMW)
		registerEndpoint(r, ep.Method, ep.Endpoint, handler)
	}

	// Register AI proxy handler if configured.
	if o.aiHandler != nil {
		r.Post("/v1/chat/completions", o.aiHandler.ServeHTTP)
	}

	return r, nil
}

// buildServiceConfig converts a domain GatewayConfig into a Lura ServiceConfig.
func buildServiceConfig(gc *domain.GatewayConfig, listenAddr string) (*luraconfig.ServiceConfig, error) {
	sc := &luraconfig.ServiceConfig{
		Name:    "flexgateway",
		Version: gc.Version,
		Timeout: 30 * time.Second,
	}

	for _, route := range gc.Routes {
		if !route.IsActive {
			continue
		}

		var detail RouteConfigDetail
		if err := json.Unmarshal(route.Config, &detail); err != nil {
			return nil, fmt.Errorf("parsing route %q config: %w", route.Name, err)
		}

		// Convert wildcard paths: /api/v1/* -> /api/v1/{path}
		endpoint := convertWildcardPath(detail.Path)

		method := strings.ToUpper(detail.Method)
		if method == "" {
			method = http.MethodGet
		}

		timeout := 30 * time.Second
		if detail.Timeout != "" {
			parsed, err := time.ParseDuration(detail.Timeout)
			if err == nil {
				timeout = parsed
			}
		}

		var backends []*luraconfig.Backend
		for _, backendURL := range detail.BackendURLs {
			parsed, err := url.Parse(backendURL)
			if err != nil {
				return nil, fmt.Errorf("parsing backend URL %q for route %q: %w", backendURL, route.Name, err)
			}

			host := parsed.Scheme + "://" + parsed.Host
			urlPattern := parsed.Path
			if urlPattern == "" {
				urlPattern = "/"
			}

			backends = append(backends, &luraconfig.Backend{
				Host:       []string{host},
				URLPattern: urlPattern,
				Method:     method,
				Encoding:   "json",
				Timeout:    timeout,
			})
		}

		epConfig := &luraconfig.EndpointConfig{
			Endpoint:       endpoint,
			Method:         method,
			Backend:        backends,
			Timeout:        timeout,
			OutputEncoding: "json",
			HeadersToPass:  detail.Headers,
		}

		sc.Endpoints = append(sc.Endpoints, epConfig)
	}

	return sc, nil
}

// convertWildcardPath converts /api/v1/* to /api/v1/{path} for chi router compatibility.
func convertWildcardPath(path string) string {
	if strings.HasSuffix(path, "/*") {
		return strings.TrimSuffix(path, "/*") + "/*"
	}
	return path
}

// buildEndpointHandler creates an HTTP handler that proxies requests through the
// block chain to the configured backends with round-robin load balancing.
func buildEndpointHandler(ep *luraconfig.EndpointConfig, chainMW proxy.Middleware) http.HandlerFunc {
	if len(ep.Backend) == 0 {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no backends configured", http.StatusBadGateway)
		}
	}

	// Build round-robin backend selector
	backends := make([]*luraconfig.Backend, len(ep.Backend))
	copy(backends, ep.Backend)

	var counter atomic.Uint64

	// Shared HTTP client with connection pooling (reused across requests).
	sharedTransport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}

	// Create the backend proxy (round-robin across backends)
	backendProxy := func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
		idx := counter.Add(1) % uint64(len(backends))

		backend := backends[idx]
		host := backend.Host[0]

		targetURL := host + backend.URLPattern

		httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, req.Body)
		if err != nil {
			return nil, fmt.Errorf("creating upstream request: %w", err)
		}

		// Copy headers from original request
		for k, v := range req.Headers {
			httpReq.Header[k] = v
		}

		client := &http.Client{
			Transport: sharedTransport,
			Timeout:   backend.Timeout,
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("calling upstream %s: %w", host, err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := readResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("reading upstream response: %w", err)
		}

		// Parse JSON response data
		data := make(map[string]interface{})
		if len(body) > 0 {
			if jsonErr := json.Unmarshal(body, &data); jsonErr != nil {
				// If not JSON, store raw body as "content"
				data["content"] = string(body)
			}
		}

		return &proxy.Response{
			Data:       data,
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
			},
		}, nil
	}

	// Wrap the backend proxy with the block chain middleware
	finalProxy := chainMW(backendProxy)

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		proxyReq := &proxy.Request{
			Method:  r.Method,
			URL:     r.URL,
			Path:    r.URL.Path,
			Query:   r.URL.Query(),
			Headers: r.Header,
			Body:    r.Body,
		}

		resp, err := finalProxy(ctx, proxyReq)
		if err != nil {
			var pe *domain.ProblemError
			if errors.As(err, &pe) {
				w.Header().Set("Content-Type", "application/problem+json")
				body, marshalErr := json.Marshal(pe)
				if marshalErr != nil {
					http.Error(w, "encoding problem error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(pe.StatusCode())
				_, _ = w.Write(body)
				return
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if resp == nil {
			http.Error(w, "empty response from upstream", http.StatusBadGateway)
			return
		}

		// Write response headers from upstream
		for k, v := range resp.Metadata.Headers {
			for _, val := range v {
				w.Header().Add(k, val)
			}
		}

		statusCode := resp.Metadata.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		// Marshal data back to JSON
		jsonData, err := json.Marshal(resp.Data)
		if err != nil {
			http.Error(w, "encoding response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write(jsonData)
	}
}

// registerEndpoint registers an HTTP handler on the chi router for the given method and path.
func registerEndpoint(r chi.Router, method, path string, handler http.HandlerFunc) {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		r.Get(path, handler)
	case http.MethodPost:
		r.Post(path, handler)
	case http.MethodPut:
		r.Put(path, handler)
	case http.MethodPatch:
		r.Patch(path, handler)
	case http.MethodDelete:
		r.Delete(path, handler)
	default:
		r.Get(path, handler)
	}
}

// readResponseBody reads the entire response body and returns it as bytes.
func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}
	return io.ReadAll(resp.Body)
}

func notFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}
