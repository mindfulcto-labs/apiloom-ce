package cors

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeHeaders builds a map[string][]string from key-value pairs for proxy.Request.Headers.
func makeHeaders(kvs ...string) map[string][]string {
	h := http.Header{}
	for i := 0; i+1 < len(kvs); i += 2 {
		h.Set(kvs[i], kvs[i+1])
	}
	return h
}

// getHeader retrieves a header value from map[string][]string using http.Header.Get semantics.
func getHeader(headers map[string][]string, key string) string {
	return http.Header(headers).Get(key)
}

func TestCORSConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  CORSConfig
		wantErr bool
	}{
		{
			name:    "empty config is valid (restrictive default)",
			config:  CORSConfig{},
			wantErr: false,
		},
		{
			name: "explicit origins valid",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
			},
			wantErr: false,
		},
		{
			name: "wildcard + credentials is forbidden",
			config: CORSConfig{
				AllowAllOrigins:  true,
				AllowCredentials: true,
			},
			wantErr: true,
		},
		{
			name: "wildcard without credentials is valid",
			config: CORSConfig{
				AllowAllOrigins: true,
			},
			wantErr: false,
		},
		{
			name: "credentials without wildcard is valid",
			config: CORSConfig{
				AllowedOrigins:   []string{"https://example.com"},
				AllowCredentials: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCORSBlock_Name(t *testing.T) {
	t.Parallel()
	block, err := NewCORSBlock(CORSConfig{})
	require.NoError(t, err)
	assert.Equal(t, "cors", block.Name())
}

func TestCORSBlock_Defaults(t *testing.T) {
	t.Parallel()
	block, err := NewCORSBlock(CORSConfig{})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET", "POST", "HEAD"}, block.config.AllowedMethods)
	assert.Equal(t, []string{"Origin", "Content-Type", "Accept", "Authorization"}, block.config.AllowedHeaders)
	assert.Equal(t, 300, block.config.MaxAge)
	// Restrictive default: no origins allowed
	assert.Empty(t, block.config.AllowedOrigins)
}

func TestCORSBlock_PreflightAllowedOrigin(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		MaxAge:         600,
	})
	require.NoError(t, err)

	nextCalled := false
	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		nextCalled = true
		return &proxy.Response{IsComplete: true}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method: http.MethodOptions,
		Headers: makeHeaders(
			"Origin", "https://example.com",
			"Access-Control-Request-Method", "POST",
		),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Preflight should NOT call next proxy (short-circuit)
	assert.False(t, nextCalled, "preflight should not call next proxy")

	// Should have CORS headers
	assert.Equal(t, "https://example.com", getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
	assert.Contains(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Methods"), "POST")
	assert.Equal(t, "600", getHeader(resp.Metadata.Headers, "Access-Control-Max-Age"))
	assert.Equal(t, 200, resp.Metadata.StatusCode)
}

func TestCORSBlock_PreflightDisallowedOrigin(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	})
	require.NoError(t, err)

	nextCalled := false
	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		nextCalled = true
		return &proxy.Response{IsComplete: true}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method: http.MethodOptions,
		Headers: makeHeaders(
			"Origin", "https://evil.com",
			"Access-Control-Request-Method", "POST",
		),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should not call next proxy for preflight
	assert.False(t, nextCalled)

	// No CORS headers for disallowed origin
	assert.Empty(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_NormalRequestAllowedOrigin(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Data:       map[string]interface{}{"key": "value"},
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://example.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "https://example.com", getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_NormalRequestDisallowedOrigin(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Data:       map[string]interface{}{"key": "value"},
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://evil.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// No CORS headers for disallowed origin
	assert.Empty(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_NormalRequestNoOrigin(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Data:       map[string]interface{}{"key": "value"},
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: map[string][]string{},
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// No CORS headers when no Origin header
	assert.Empty(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_AllowCredentials(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://example.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "true", getHeader(resp.Metadata.Headers, "Access-Control-Allow-Credentials"))
}

func TestCORSBlock_AllowAllOriginsExplicit(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins:  []string{"*"},
		AllowAllOrigins: true,
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://any-origin.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "*", getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_WildcardIgnoredWithoutExplicitFlag(t *testing.T) {
	t.Parallel()

	// Even if AllowedOrigins contains "*", it should NOT match without AllowAllOrigins=true
	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"*"},
		// AllowAllOrigins is false (default)
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://any-origin.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// "*" in AllowedOrigins without AllowAllOrigins=true should NOT allow any origin
	assert.Empty(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_RestrictiveDefaultNoOrigins(t *testing.T) {
	t.Parallel()

	// Default config with no origins = nothing allowed
	block, err := NewCORSBlock(CORSConfig{})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodGet,
		Headers: makeHeaders("Origin", "https://example.com"),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Empty(t, getHeader(resp.Metadata.Headers, "Access-Control-Allow-Origin"))
}

func TestCORSBlock_PreflightMaxAge(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		MaxAge:         3600,
	})
	require.NoError(t, err)

	mw := block.Middleware()
	p := mw(proxy.NoopProxy)

	req := &proxy.Request{
		Method: http.MethodOptions,
		Headers: makeHeaders(
			"Origin", "https://example.com",
			"Access-Control-Request-Method", "GET",
		),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "3600", getHeader(resp.Metadata.Headers, "Access-Control-Max-Age"))
}

func TestCORSBlock_PreflightAllowedHeaders(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		AllowedHeaders: []string{"X-Custom", "X-Another"},
	})
	require.NoError(t, err)

	mw := block.Middleware()
	p := mw(proxy.NoopProxy)

	req := &proxy.Request{
		Method: http.MethodOptions,
		Headers: makeHeaders(
			"Origin", "https://example.com",
			"Access-Control-Request-Method", "GET",
			"Access-Control-Request-Headers", "X-Custom",
		),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	allowedHeaders := getHeader(resp.Metadata.Headers, "Access-Control-Allow-Headers")
	assert.Contains(t, allowedHeaders, "X-Custom")
}

// Verify passthrough of body on normal requests
func TestCORSBlock_PassthroughBody(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		if req.Body != nil {
			body, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				return nil, readErr
			}
			return &proxy.Response{
				IsComplete: true,
				Data:       map[string]interface{}{"body": string(body)},
				Metadata: proxy.Metadata{
					StatusCode: 200,
					Headers:    http.Header{},
				},
			}, nil
		}
		return &proxy.Response{
			IsComplete: true,
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodPost,
		Headers: makeHeaders("Origin", "https://example.com"),
		Body:    io.NopCloser(strings.NewReader(`{"hello":"world"}`)),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, `{"hello":"world"}`, resp.Data["body"])
}

// Verify the block implements the Block interface
func TestCORSBlock_ImplementsBlockInterface(t *testing.T) {
	t.Parallel()

	block, err := NewCORSBlock(CORSConfig{})
	require.NoError(t, err)

	// Compile-time interface check
	var _ interface {
		Name() string
		Middleware() proxy.Middleware
	} = block
	_ = fmt.Sprintf("%T", block)
}
