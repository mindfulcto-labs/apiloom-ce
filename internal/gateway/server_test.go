package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// mockConfigProvider implements ConfigProvider for tests.
type mockConfigProvider struct {
	config *domain.GatewayConfig
}

func (m *mockConfigProvider) Get() *domain.GatewayConfig {
	return m.config
}

// routeConfigJSON builds a route config JSON with the given parameters.
func routeConfigJSON(t *testing.T, path, method string, backendURLs []string) json.RawMessage {
	t.Helper()
	cfg := RouteConfigDetail{
		Path:        path,
		Method:      method,
		BackendURLs: backendURLs,
		Timeout:     "30s",
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	return data
}

func TestBlockInterface(t *testing.T) {
	t.Parallel()

	t.Run("noopBlock satisfies Block interface", func(t *testing.T) {
		t.Parallel()
		var b Block = &noopBlock{}
		assert.Equal(t, "noop", b.Name())
		assert.NotNil(t, b.Middleware())
	})
}

func TestBlockChain(t *testing.T) {
	t.Parallel()

	t.Run("empty chain returns passthrough middleware", func(t *testing.T) {
		t.Parallel()
		bc := NewBlockChain()
		mw := bc.Middleware()
		assert.NotNil(t, mw)
	})

	t.Run("chain preserves block order", func(t *testing.T) {
		t.Parallel()
		var order []string
		b1 := &orderTrackingBlock{name: "first", order: &order}
		b2 := &orderTrackingBlock{name: "second", order: &order}

		bc := NewBlockChain(b1, b2)
		assert.Equal(t, 2, bc.Len())
		assert.Equal(t, "first", bc.blocks[0].Name())
		assert.Equal(t, "second", bc.blocks[1].Name())
	})

	t.Run("middleware executes blocks in order", func(t *testing.T) {
		t.Parallel()
		var order []string
		b1 := &orderTrackingBlock{name: "first", order: &order}
		b2 := &orderTrackingBlock{name: "second", order: &order}

		bc := NewBlockChain(b1, b2)
		mw := bc.Middleware()

		backend := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
			order = append(order, "backend")
			return &proxy.Response{IsComplete: true}, nil
		}

		finalProxy := mw(backend)
		_, err := finalProxy(context.Background(), &proxy.Request{})
		require.NoError(t, err)

		assert.Equal(t, []string{"first", "second", "backend"}, order)
	})
}

func TestBuildServiceConfig(t *testing.T) {
	t.Parallel()

	t.Run("empty config produces no endpoints", func(t *testing.T) {
		t.Parallel()
		gc := &domain.GatewayConfig{
			Version: 1,
			Routes:  nil,
		}
		sc, err := buildServiceConfig(gc, ":8080")
		require.NoError(t, err)
		assert.Empty(t, sc.Endpoints)
	})

	t.Run("single route produces endpoint", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
		}))
		defer upstream.Close()

		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "test-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/v1/users", "GET", []string{upstream.URL}),
				},
			},
		}
		sc, err := buildServiceConfig(gc, ":8080")
		require.NoError(t, err)
		require.Len(t, sc.Endpoints, 1)
		assert.Equal(t, "/api/v1/users", sc.Endpoints[0].Endpoint)
		assert.Equal(t, "GET", sc.Endpoints[0].Method)
		require.Len(t, sc.Endpoints[0].Backend, 1)
	})

	t.Run("wildcard path preserved for chi", func(t *testing.T) {
		t.Parallel()
		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "wildcard-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/v1/*", "GET", []string{"http://localhost:9999"}),
				},
			},
		}
		sc, err := buildServiceConfig(gc, ":8080")
		require.NoError(t, err)
		require.Len(t, sc.Endpoints, 1)
		// chi uses /* for wildcard catch-all
		assert.Equal(t, "/api/v1/*", sc.Endpoints[0].Endpoint)
	})

	t.Run("inactive routes are skipped", func(t *testing.T) {
		t.Parallel()
		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "active-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/active", "GET", []string{"http://localhost:9999"}),
				},
				{
					Name:     "inactive-route",
					IsActive: false,
					Config:   routeConfigJSON(t, "/api/inactive", "GET", []string{"http://localhost:9999"}),
				},
			},
		}
		sc, err := buildServiceConfig(gc, ":8080")
		require.NoError(t, err)
		require.Len(t, sc.Endpoints, 1)
		assert.Equal(t, "/api/active", sc.Endpoints[0].Endpoint)
	})

	t.Run("multiple backends per route", func(t *testing.T) {
		t.Parallel()
		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "lb-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/balanced", "GET", []string{"http://host1:8080", "http://host2:8080"}),
				},
			},
		}
		sc, err := buildServiceConfig(gc, ":8080")
		require.NoError(t, err)
		require.Len(t, sc.Endpoints, 1)
		require.Len(t, sc.Endpoints[0].Backend, 2)
	})
}

func TestServerPathRouting(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"path":"%s"}`, r.URL.Path)
	}))
	defer upstream.Close()

	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{
				Name:     "users-route",
				IsActive: true,
				Config:   routeConfigJSON(t, "/api/v1/users", "GET", []string{upstream.URL}),
			},
		},
		LoadedAt: time.Now(),
	}

	srv := newTestServer(t, gc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServerWildcardRouting(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"matched":"%s"}`, r.URL.Path)
	}))
	defer upstream.Close()

	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{
				Name:     "wildcard-route",
				IsActive: true,
				Config:   routeConfigJSON(t, "/api/v1/*", "GET", []string{upstream.URL}),
			},
		},
		LoadedAt: time.Now(),
	}

	srv := newTestServer(t, gc)
	defer srv.Close()

	tests := []struct {
		name string
		path string
	}{
		{name: "users subpath", path: "/api/v1/users"},
		{name: "orders subpath", path: "/api/v1/orders"},
		{name: "nested subpath", path: "/api/v1/users/123/profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestServerMethodRouting(t *testing.T) {
	t.Parallel()

	getUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"method":"GET"}`)
	}))
	defer getUpstream.Close()

	postUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"method":"POST"}`)
	}))
	defer postUpstream.Close()

	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{
				Name:     "resource-get",
				IsActive: true,
				Config:   routeConfigJSON(t, "/api/v1/resource", "GET", []string{getUpstream.URL}),
			},
			{
				Name:     "resource-post",
				IsActive: true,
				Config:   routeConfigJSON(t, "/api/v1/resource", "POST", []string{postUpstream.URL}),
			},
		},
		LoadedAt: time.Now(),
	}

	srv := newTestServer(t, gc)
	defer srv.Close()

	// GET should reach getUpstream
	resp, err := http.Get(srv.URL + "/api/v1/resource")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "GET")

	// POST should reach postUpstream
	resp2, err := http.Post(srv.URL+"/api/v1/resource", "application/json", nil)
	require.NoError(t, err)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Contains(t, string(body2), "POST")
}

func TestServerRoundRobin(t *testing.T) {
	t.Parallel()

	var counter1 atomic.Int64
	var counter2 atomic.Int64

	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter1.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"backend":"1"}`)
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter2.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"backend":"2"}`)
	}))
	defer upstream2.Close()

	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{
				Name:     "lb-route",
				IsActive: true,
				Config:   routeConfigJSON(t, "/api/v1/balanced", "GET", []string{upstream1.URL, upstream2.URL}),
			},
		},
		LoadedAt: time.Now(),
	}

	srv := newTestServer(t, gc)
	defer srv.Close()

	// Send 10 requests; both backends should receive some
	for i := 0; i < 10; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/balanced")
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	c1 := counter1.Load()
	c2 := counter2.Load()
	// Both should receive at least 1 request (round-robin distribution)
	assert.Greater(t, c1, int64(0), "backend 1 should receive requests")
	assert.Greater(t, c2, int64(0), "backend 2 should receive requests")
	assert.Equal(t, int64(10), c1+c2, "total requests should be 10")
}

func TestServerEmptyConfig(t *testing.T) {
	t.Parallel()

	gc := &domain.GatewayConfig{
		Version:  1,
		Routes:   nil,
		LoadedAt: time.Now(),
	}

	// Should not panic with empty config
	srv := newTestServer(t, gc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nonexistent")
	require.NoError(t, err)
	resp.Body.Close()
	// 404 for unmatched routes
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// newTestServer creates a test HTTP server backed by the gateway handler.
func newTestServer(t *testing.T, gc *domain.GatewayConfig) *httptest.Server {
	t.Helper()

	provider := &mockConfigProvider{config: gc}
	handler, err := BuildHandler(provider, NewBlockChain())
	require.NoError(t, err)

	return httptest.NewServer(handler)
}

func TestServerProblemErrorHandler(t *testing.T) {
	t.Parallel()

	t.Run("ProblemError returns application/problem+json with correct status", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		}))
		defer upstream.Close()

		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "problem-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/v1/secured", "GET", []string{upstream.URL}),
				},
			},
			LoadedAt: time.Now(),
		}

		// Block that returns a ProblemError
		problemBlock := &problemErrorBlock{
			err: domain.NewProblemError(401, "unauthorized", "invalid API key"),
		}

		provider := &mockConfigProvider{config: gc}
		handler, err := BuildHandler(provider, NewBlockChain(problemBlock))
		require.NoError(t, err)

		srv := httptest.NewServer(handler)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/secured")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var problem map[string]interface{}
		err = json.Unmarshal(body, &problem)
		require.NoError(t, err)
		assert.Equal(t, "urn:flexgateway:error:unauthorized", problem["type"])
		assert.Equal(t, float64(401), problem["status"])
		assert.Equal(t, "invalid API key", problem["detail"])
	})

	t.Run("non-ProblemError returns 502 Bad Gateway", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		}))
		defer upstream.Close()

		gc := &domain.GatewayConfig{
			Version: 1,
			Routes: []domain.RouteConfig{
				{
					Name:     "error-route",
					IsActive: true,
					Config:   routeConfigJSON(t, "/api/v1/failing", "GET", []string{upstream.URL}),
				},
			},
			LoadedAt: time.Now(),
		}

		errorBlock := &genericErrorBlock{
			err: fmt.Errorf("connection refused"),
		}

		provider := &mockConfigProvider{config: gc}
		handler, err := BuildHandler(provider, NewBlockChain(errorBlock))
		require.NoError(t, err)

		srv := httptest.NewServer(handler)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/failing")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}

// problemErrorBlock is a test Block that returns a ProblemError.
type problemErrorBlock struct {
	err *domain.ProblemError
}

func (b *problemErrorBlock) Name() string { return "problem-error" }
func (b *problemErrorBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
			return nil, b.err
		}
	}
}

// genericErrorBlock is a test Block that returns a generic error.
type genericErrorBlock struct {
	err error
}

func (b *genericErrorBlock) Name() string { return "generic-error" }
func (b *genericErrorBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
			return nil, b.err
		}
	}
}

// noopBlock is a test Block that does nothing.
type noopBlock struct{}

func (b *noopBlock) Name() string { return "noop" }
func (b *noopBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) > 0 {
			return next[0]
		}
		return proxy.NoopProxy
	}
}

// orderTrackingBlock tracks the order in which blocks are executed.
type orderTrackingBlock struct {
	name  string
	order *[]string
}

func (b *orderTrackingBlock) Name() string { return b.name }
func (b *orderTrackingBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			*b.order = append(*b.order, b.name)
			if len(next) > 0 {
				return next[0](ctx, req)
			}
			return &proxy.Response{IsComplete: true}, nil
		}
	}
}
