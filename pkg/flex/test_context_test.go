package flex

import (
	"context"
	"errors"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

func TestTestContext_NoOpBlock(t *testing.T) {
	t.Parallel()

	tc := NewTestContext()
	block := &BaseBlock{BlockName: "noop"}
	req := NewTestRequest("GET", "/api/health")

	resp, err := tc.ExecuteRequest(block, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsComplete)
}

func TestTestContext_HeaderModification(t *testing.T) {
	t.Parallel()

	// A block that adds a header
	block := &headerBlock{
		BaseBlock: BaseBlock{BlockName: "header-adder"},
		key:       "X-Plugin",
		value:     "test",
	}

	tc := NewTestContext()
	req := NewTestRequest("GET", "/api/test")

	resp, err := tc.ExecuteRequest(block, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	// The header should have been set on the request that reached the backend
	assert.Equal(t, []string{"test"}, req.Headers["X-Plugin"])
}

func TestTestContext_RejectBlock(t *testing.T) {
	t.Parallel()

	block := &rejectBlock{
		BaseBlock: BaseBlock{BlockName: "rejector"},
		status:    403,
		errType:   "forbidden",
		detail:    "not allowed",
	}

	tc := NewTestContext()
	req := NewTestRequest("POST", "/api/admin")

	resp, err := tc.ExecuteRequest(block, req)
	require.Error(t, err)
	assert.Nil(t, resp)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, 403, pe.Status)
	assert.Contains(t, pe.Detail, "not allowed")
}

func TestTestContext_WithBackendResponse(t *testing.T) {
	t.Parallel()

	customResp := NewTestResponse(200, map[string]interface{}{"message": "hello"})
	tc := NewTestContext().WithBackendResponse(customResp)

	block := &BaseBlock{BlockName: "passthrough"}
	req := NewTestRequest("GET", "/api/data")

	resp, err := tc.ExecuteRequest(block, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "hello", resp.Data["message"])
}

func TestTestContext_WithBackendError(t *testing.T) {
	t.Parallel()

	backendErr := errors.New("upstream timeout")
	tc := NewTestContext().WithBackendError(backendErr)

	block := &BaseBlock{BlockName: "passthrough"}
	req := NewTestRequest("GET", "/api/slow")

	resp, err := tc.ExecuteRequest(block, req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, "upstream timeout", err.Error())
}

func TestTestContext_ExecuteRequestWithContext(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "custom-value")

	// A block that checks context
	block := &contextCheckBlock{
		BaseBlock: BaseBlock{BlockName: "ctx-checker"},
		key:       ctxKey{},
	}

	tc := NewTestContext()
	req := NewTestRequest("GET", "/api/test")

	resp, err := tc.ExecuteRequestWithContext(ctx, block, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "custom-value", resp.Data["ctx_value"])
}

func TestNewTestRequest(t *testing.T) {
	t.Parallel()

	req := NewTestRequest("POST", "/api/users")
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "/api/users", req.Path)
	require.NotNil(t, req.Headers)
	require.NotNil(t, req.Params)
	require.NotNil(t, req.Query)
}

func TestNewTestResponse(t *testing.T) {
	t.Parallel()

	data := map[string]interface{}{"id": "123", "name": "test"}
	resp := NewTestResponse(200, data)
	assert.True(t, resp.IsComplete)
	assert.Equal(t, "123", resp.Data["id"])
	assert.Equal(t, "test", resp.Data["name"])
}

func TestNewTestResponse_NilData(t *testing.T) {
	t.Parallel()

	resp := NewTestResponse(204, nil)
	assert.True(t, resp.IsComplete)
	assert.Nil(t, resp.Data)
}

// --- Test helper blocks ---

// headerBlock adds a header to the request.
type headerBlock struct {
	BaseBlock
	key   string
	value string
}

func (b *headerBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		backend := next[0]
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			rc := NewRequestContext(req)
			rc.WithHeader(b.key, b.value)
			return backend(ctx, req)
		}
	}
}

// rejectBlock rejects the request with a ProblemError.
type rejectBlock struct {
	BaseBlock
	status  int
	errType string
	detail  string
}

func (b *rejectBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			rc := NewRequestContext(req)
			return nil, rc.RejectRequest(b.status, b.errType, b.detail)
		}
	}
}

// contextCheckBlock reads a value from context and puts it in the response.
type contextCheckBlock struct {
	BaseBlock
	key interface{}
}

func (b *contextCheckBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		backend := next[0]
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			resp, err := backend(ctx, req)
			if err != nil {
				return nil, err
			}
			if val := ctx.Value(b.key); val != nil {
				if resp.Data == nil {
					resp.Data = make(map[string]interface{})
				}
				resp.Data["ctx_value"] = val
			}
			return resp, nil
		}
	}
}
