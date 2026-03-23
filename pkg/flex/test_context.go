package flex

import (
	"context"
	"net/url"

	"github.com/luraproject/lura/v2/proxy"
)

// TestContext provides a mock pipeline for testing Block implementations
// in standard go test without any gateway infrastructure.
//
// Usage:
//
//	tc := flex.NewTestContext()
//	req := flex.NewTestRequest("GET", "/api/test")
//	resp, err := tc.ExecuteRequest(myBlock, req)
type TestContext struct {
	backendResponse *proxy.Response
	backendError    error
}

// NewTestContext creates a TestContext with a default 200 OK empty response.
func NewTestContext() *TestContext {
	return &TestContext{
		backendResponse: &proxy.Response{
			Data:       make(map[string]interface{}),
			IsComplete: true,
		},
	}
}

// WithBackendResponse configures the mock backend to return the given response.
func (tc *TestContext) WithBackendResponse(resp *proxy.Response) *TestContext {
	tc.backendResponse = resp
	return tc
}

// WithBackendError configures the mock backend to return the given error.
func (tc *TestContext) WithBackendError(err error) *TestContext {
	tc.backendError = err
	return tc
}

// ExecuteRequest runs the block's middleware against a mock backend and returns
// the response. Uses context.Background().
func (tc *TestContext) ExecuteRequest(block Block, req *proxy.Request) (*proxy.Response, error) {
	return tc.ExecuteRequestWithContext(context.Background(), block, req)
}

// ExecuteRequestWithContext runs the block's middleware with a custom context.
func (tc *TestContext) ExecuteRequestWithContext(ctx context.Context, block Block, req *proxy.Request) (*proxy.Response, error) {
	// Create mock backend proxy
	backend := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		if tc.backendError != nil {
			return nil, tc.backendError
		}
		return tc.backendResponse, nil
	}

	// Wrap with the block's middleware
	mw := block.Middleware()
	wrapped := mw(backend)

	return wrapped(ctx, req)
}

// NewTestRequest builds a minimal valid proxy.Request for testing.
func NewTestRequest(method, path string) *proxy.Request {
	u, _ := url.Parse("http://localhost" + path)
	return &proxy.Request{
		Method:  method,
		URL:     u,
		Path:    path,
		Query:   u.Query(),
		Headers: make(map[string][]string),
		Params:  make(map[string]string),
	}
}

// NewTestResponse builds a proxy.Response for configuring test backends.
func NewTestResponse(statusCode int, data map[string]interface{}) *proxy.Response {
	return &proxy.Response{
		Data:       data,
		IsComplete: true,
		Metadata: proxy.Metadata{
			StatusCode: statusCode,
		},
	}
}
