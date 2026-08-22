package benchmarks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mindfulcto-labs/apiloom-ce/internal/block/ratelimit"
	"github.com/mindfulcto-labs/apiloom-ce/internal/block/security"
	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/gateway"
)

// mockConfigProvider implements gateway.ConfigProvider for benchmarks.
type mockConfigProvider struct {
	config *domain.GatewayConfig
}

func (m *mockConfigProvider) Get() *domain.GatewayConfig {
	return m.config
}

// benchBackend returns an httptest.Server that responds with a 1KB JSON body.
func benchBackend(tb testing.TB) *httptest.Server {
	tb.Helper()
	body := generateJSONBody(1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	tb.Cleanup(srv.Close)
	return srv
}

// generateJSONBody produces a JSON byte slice of approximately targetBytes.
func generateJSONBody(targetBytes int) []byte {
	data := map[string]interface{}{
		"id":     1,
		"name":   "benchmark-resource",
		"status": "active",
		"metadata": map[string]interface{}{
			"region":  "us-east-1",
			"tier":    "premium",
			"version": 3,
		},
		"tags":    []string{"api", "gateway", "benchmark", "performance"},
		"payload": strings.Repeat("x", targetBytes/2),
	}
	b, _ := json.Marshal(data)
	return b
}

// benchRouteConfig builds a route config JSON.
func benchRouteConfig(tb testing.TB, path, method string, backendURLs []string) json.RawMessage {
	tb.Helper()
	cfg := struct {
		Path        string   `json:"path"`
		Method      string   `json:"method"`
		BackendURLs []string `json:"backend_urls"`
		Timeout     string   `json:"timeout,omitempty"`
	}{
		Path:        path,
		Method:      method,
		BackendURLs: backendURLs,
		Timeout:     "30s",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		tb.Fatalf("marshaling bench route config: %v", err)
	}
	return data
}

// latencyRecorder tracks request latencies for p50/p99 reporting.
type latencyRecorder struct {
	mu       sync.Mutex
	samples  []time.Duration
	maxKeep  int
	overflow atomic.Int64
}

func newLatencyRecorder(maxKeep int) *latencyRecorder {
	return &latencyRecorder{
		samples: make([]time.Duration, 0, maxKeep),
		maxKeep: maxKeep,
	}
}

func (r *latencyRecorder) record(d time.Duration) {
	r.mu.Lock()
	if len(r.samples) < r.maxKeep {
		r.samples = append(r.samples, d)
	} else {
		r.overflow.Add(1)
	}
	r.mu.Unlock()
}

func (r *latencyRecorder) percentile(p float64) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.samples) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(r.samples))
	copy(sorted, r.samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func reportLatency(b *testing.B, lr *latencyRecorder) {
	b.Helper()
	p50 := lr.percentile(0.50)
	p99 := lr.percentile(0.99)
	b.ReportMetric(float64(p50.Microseconds()), "p50-us")
	b.ReportMetric(float64(p99.Microseconds()), "p99-us")
}

// mockBackendProxy returns a Lura proxy that simulates a 1KB JSON backend response
// with zero network overhead, isolating measurement to gateway middleware cost.
func mockBackendProxy() proxy.Proxy {
	respData := map[string]interface{}{
		"id":     float64(1),
		"name":   "benchmark-resource",
		"status": "active",
		"metadata": map[string]interface{}{
			"region":  "us-east-1",
			"tier":    "premium",
			"version": float64(3),
		},
		"tags": []interface{}{"api", "gateway", "benchmark", "performance"},
	}
	return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			Data:       respData,
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    map[string][]string{"Content-Type": {"application/json"}},
			},
		}, nil
	}
}

// jwtAuthBlock simulates JWT validation overhead without external JWKS calls.
// It parses and HMAC-validates a pre-signed token on every request.
type jwtAuthBlock struct {
	secret []byte
	token  string
}

func newJWTAuthBlock() *jwtAuthBlock {
	secret := []byte("bench-secret-key-32bytes-long!!")
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		`{"sub":"bench-user","iss":"flexgateway","exp":9999999999,"iat":1700000000}`,
	))
	sigInput := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sigInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := sigInput + "." + sig

	return &jwtAuthBlock{secret: secret, token: token}
}

func (b *jwtAuthBlock) Name() string { return "auth-jwt-bench" }

func (b *jwtAuthBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			authHeader := ""
			if vals, ok := req.Headers["Authorization"]; ok && len(vals) > 0 {
				authHeader = vals[0]
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				token = b.token
			}

			parts := strings.SplitN(token, ".", 3)
			if len(parts) != 3 {
				return nil, fmt.Errorf("invalid token format")
			}
			mac := hmac.New(sha256.New, b.secret)
			mac.Write([]byte(parts[0] + "." + parts[1]))
			expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
			if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
				return nil, fmt.Errorf("invalid signature")
			}

			_, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil {
				return nil, fmt.Errorf("decoding payload: %w", err)
			}

			return nextProxy(ctx, req)
		}
	}
}

// aiProxyBlock simulates AI proxy overhead: token counting and budget check.
type aiProxyBlock struct{}

func (b *aiProxyBlock) Name() string { return "ai-proxy-bench" }

func (b *aiProxyBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}
		budgetUsed := &atomic.Int64{}
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			// Simulate token counting on request body.
			if req.Body != nil {
				buf := make([]byte, 512)
				n, _ := req.Body.Read(buf)
				if n > 0 {
					_ = len(strings.Fields(string(buf[:n])))
				}
			}

			// Simulate budget check.
			budgetUsed.Add(1)
			if budgetUsed.Load() > 1_000_000_000 {
				return nil, fmt.Errorf("budget exceeded")
			}

			return nextProxy(ctx, req)
		}
	}
}

// BenchmarkProxyPassthrough measures pure gateway overhead: router -> single backend
// with real HTTP round-trip via httptest.Server. Target: high throughput baseline.
func BenchmarkProxyPassthrough(b *testing.B) {
	backend := benchBackend(b)

	cfg := benchRouteConfig(b, "/api/v1/bench", "GET", []string{backend.URL})
	gc := &domain.GatewayConfig{
		Version: 1,
		Routes: []domain.RouteConfig{
			{Name: "bench-passthrough", IsActive: true, Config: cfg},
		},
		LoadedAt: time.Now(),
	}

	provider := &mockConfigProvider{config: gc}
	handler, err := gateway.BuildHandler(provider, gateway.NewBlockChain())
	if err != nil {
		b.Fatalf("building handler: %v", err)
	}

	lr := newLatencyRecorder(100_000)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/bench", nil)
			rec := httptest.NewRecorder()
			start := time.Now()
			handler.ServeHTTP(rec, req)
			lr.record(time.Since(start))
			if rec.Code != http.StatusOK {
				b.Errorf("unexpected status: %d", rec.Code)
			}
		}
	})

	reportLatency(b, lr)
}

// BenchmarkAuthRateLimit measures JWT validation + sliding window rate limit overhead
// at the Lura proxy layer. Uses a mock backend proxy to isolate middleware cost.
// Target: 100K+ RPS.
func BenchmarkAuthRateLimit(b *testing.B) {
	authBlock := newJWTAuthBlock()

	rlBlock, err := ratelimit.NewRateLimitBlock(ratelimit.Config{
		Enabled: true,
		Rate:    1_000_000,
		Burst:   2_000_000,
	})
	if err != nil {
		b.Fatalf("creating rate limit block: %v", err)
	}

	chain := gateway.NewBlockChain(authBlock, rlBlock)
	mw := chain.Middleware()
	finalProxy := mw(mockBackendProxy())

	token := authBlock.token
	lr := newLatencyRecorder(100_000)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := &proxy.Request{
				Method:  "GET",
				Path:    "/api/v1/secure",
				Headers: map[string][]string{"Authorization": {"Bearer " + token}},
			}
			start := time.Now()
			resp, err := finalProxy(context.Background(), req)
			lr.record(time.Since(start))
			if err != nil {
				b.Fatalf("proxy error: %v", err)
			}
			if resp == nil || !resp.IsComplete {
				b.Fatal("incomplete response")
			}
		}
	})

	reportLatency(b, lr)
}

// BenchmarkAIProxy measures AI proxy overhead: token counting + budget check
// at the Lura proxy layer. Target: 80K+ RPS.
func BenchmarkAIProxy(b *testing.B) {
	aiBlock := &aiProxyBlock{}
	chain := gateway.NewBlockChain(aiBlock)
	mw := chain.Middleware()
	finalProxy := mw(mockBackendProxy())

	lr := newLatencyRecorder(100_000)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := &proxy.Request{
				Method:  "POST",
				Path:    "/v1/chat/completions",
				Headers: map[string][]string{"Content-Type": {"application/json"}},
				Body:    http.NoBody,
			}
			start := time.Now()
			resp, err := finalProxy(context.Background(), req)
			lr.record(time.Since(start))
			if err != nil {
				b.Fatalf("proxy error: %v", err)
			}
			if resp == nil || !resp.IsComplete {
				b.Fatal("incomplete response")
			}
		}
	})

	reportLatency(b, lr)
}

// BenchmarkSecurityBlock measures full security pipeline overhead:
// OWASP checks + DLP scanning + secret scanning at the Lura proxy layer.
func BenchmarkSecurityBlock(b *testing.B) {
	reg := prometheus.NewRegistry()
	secBlock, err := security.New(security.Config{
		OWASPEnabled:      true,
		DLPEnabled:        true,
		SecretScanEnabled: true,
		DLPStrategy:       "redact",
		OWASPRules: security.OWASPRuleConfig{
			InjectionEnabled:      true,
			MassAssignmentEnabled: true,
			AllowedFields:         []string{"id", "name", "status", "metadata", "tags", "payload"},
			ExcessiveDataEnabled:  true,
			MaxResponseBytes:      1 << 20,
			BrokenAuthEnabled:     false,
			SSRFEnabled:           true,
		},
	}, reg)
	if err != nil {
		b.Fatalf("creating security block: %v", err)
	}

	chain := gateway.NewBlockChain(secBlock)
	mw := chain.Middleware()
	finalProxy := mw(mockBackendProxy())

	lr := newLatencyRecorder(100_000)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := &proxy.Request{
				Method:  "GET",
				Path:    "/api/v1/protected",
				Headers: make(map[string][]string),
			}
			start := time.Now()
			resp, err := finalProxy(context.Background(), req)
			lr.record(time.Since(start))
			if err != nil {
				b.Fatalf("proxy error: %v", err)
			}
			if resp == nil || !resp.IsComplete {
				b.Fatal("incomplete response")
			}
		}
	})

	reportLatency(b, lr)
}
