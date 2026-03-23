package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// BenchmarkProxyHandler measures per-request overhead through the proxy handler
// with a mock backend returning a JSON response. Uses b.RunParallel for
// concurrent throughput measurement.
func BenchmarkProxyHandler(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","data":{"id":1,"name":"test"}}`)
	}))
	defer backend.Close()

	cfg := benchRouteConfig(b, "/api/v1/bench", "GET", []string{backend.URL})
	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{
				Name:     "bench-route",
				IsActive: true,
				Config:   cfg,
			},
		},
		LoadedAt: time.Now(),
	}

	provider := &mockConfigProvider{config: gc}
	handler, err := BuildHandler(provider, NewBlockChain())
	if err != nil {
		b.Fatalf("building handler: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/bench", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Errorf("unexpected status: %d", rec.Code)
			}
		}
	})
}

// BenchmarkBlockChain measures middleware composition cost with a chain of
// three blocks (simulating auth, rate limit, CORS) around a backend proxy.
func BenchmarkBlockChain(b *testing.B) {
	blocks := []Block{
		&passthroughBlock{blockName: "auth-check"},
		&passthroughBlock{blockName: "rate-limit-check"},
		&passthroughBlock{blockName: "cors-check"},
	}

	chain := NewBlockChain(blocks...)
	mw := chain.Middleware()

	backendProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			Data:       map[string]interface{}{"ok": true},
			IsComplete: true,
			Metadata:   proxy.Metadata{StatusCode: 200},
		}, nil
	}

	finalProxy := mw(backendProxy)
	req := &proxy.Request{
		Method: "GET",
		Path:   "/api/v1/test",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, err := finalProxy(context.Background(), req)
		if err != nil {
			b.Fatalf("proxy error: %v", err)
		}
	}
}

// BenchmarkRouteMatch measures chi router lookup performance against 100
// configured routes.
func BenchmarkRouteMatch(b *testing.B) {
	routes := make([]domain.RouteConfig, 100)
	for i := range 100 {
		routes[i] = domain.RouteConfig{
			Name:     fmt.Sprintf("route-%03d", i),
			IsActive: true,
			Config:   benchRouteConfig(b, fmt.Sprintf("/api/v%d/resource%d", i/10, i), "GET", []string{"http://localhost:19999"}),
		}
	}

	gc := &domain.GatewayConfig{
		Version:  1,
		Routes:   routes,
		LoadedAt: time.Now(),
	}

	provider := &mockConfigProvider{config: gc}
	handler, err := BuildHandler(provider, NewBlockChain())
	if err != nil {
		b.Fatalf("building handler: %v", err)
	}

	// Pick a route in the middle of the table to avoid best-case ordering.
	targetPath := "/api/v5/resource50"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, targetPath, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkJSONResponse measures JSON marshaling overhead for a typical proxy
// response payload.
func BenchmarkJSONResponse(b *testing.B) {
	data := map[string]interface{}{
		"id":         1,
		"name":       "test-resource",
		"status":     "active",
		"created_at": "2026-01-01T00:00:00Z",
		"metadata": map[string]interface{}{
			"region":  "us-east-1",
			"tier":    "premium",
			"version": 3,
		},
		"tags": []string{"api", "gateway", "benchmark"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatalf("marshal error: %v", err)
		}
	}
}

// benchRouteConfig builds a route config JSON for benchmarks.
func benchRouteConfig(b testing.TB, path, method string, backendURLs []string) json.RawMessage {
	b.Helper()
	cfg := RouteConfigDetail{
		Path:        path,
		Method:      method,
		BackendURLs: backendURLs,
		Timeout:     "30s",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		b.Fatalf("marshaling bench route config: %v", err)
	}
	return data
}

// passthroughBlock is a benchmark-only Block that passes requests through
// with minimal overhead, simulating the middleware dispatch path without
// actual business logic.
type passthroughBlock struct {
	blockName string
}

func (b *passthroughBlock) Name() string { return b.blockName }
func (b *passthroughBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			if len(next) > 0 {
				return next[0](ctx, req)
			}
			return &proxy.Response{IsComplete: true}, nil
		}
	}
}
