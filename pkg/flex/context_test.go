package flex

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

func TestRequestContext_WithHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		value    string
		existing map[string][]string
		wantLen  int
	}{
		{
			name:     "set new header",
			key:      "X-Custom",
			value:    "test",
			existing: nil,
			wantLen:  1,
		},
		{
			name:     "overwrite existing header",
			key:      "X-Custom",
			value:    "new",
			existing: map[string][]string{"X-Custom": {"old"}},
			wantLen:  1,
		},
		{
			name:     "add to existing headers",
			key:      "X-New",
			value:    "val",
			existing: map[string][]string{"X-Existing": {"keep"}},
			wantLen:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := &proxy.Request{Headers: tt.existing}
			rc := NewRequestContext(req)
			rc.WithHeader(tt.key, tt.value)

			assert.Len(t, req.Headers, tt.wantLen)
			assert.Equal(t, []string{tt.value}, req.Headers[tt.key])
		})
	}
}

func TestRequestContext_WithBody(t *testing.T) {
	t.Parallel()

	req := &proxy.Request{}
	rc := NewRequestContext(req)

	body := io.NopCloser(strings.NewReader("hello"))
	rc.WithBody(body)

	require.NotNil(t, req.Body)
	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestRequestContext_RejectRequest(t *testing.T) {
	t.Parallel()

	req := &proxy.Request{}
	rc := NewRequestContext(req)

	err := rc.RejectRequest(403, "forbidden", "Access denied")
	require.Error(t, err)

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, 403, pe.Status)
	assert.Contains(t, pe.Type, "forbidden")
	assert.Equal(t, "Access denied", pe.Detail)
}

func TestRequestContext_Request(t *testing.T) {
	t.Parallel()

	req := &proxy.Request{
		Method: "POST",
		Path:   "/api/test",
	}
	rc := NewRequestContext(req)
	assert.Equal(t, req, rc.Request())
}

func TestRequestContext_WithHeader_NilHeaders(t *testing.T) {
	t.Parallel()

	req := &proxy.Request{}
	rc := NewRequestContext(req)
	rc.WithHeader("X-Test", "value")

	assert.Equal(t, []string{"value"}, req.Headers["X-Test"])
}

func TestResponseContext_WithData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		value    interface{}
		existing map[string]interface{}
	}{
		{
			name:     "set new data",
			key:      "result",
			value:    "ok",
			existing: nil,
		},
		{
			name:     "overwrite existing data",
			key:      "result",
			value:    "new",
			existing: map[string]interface{}{"result": "old"},
		},
		{
			name:     "add to existing data",
			key:      "extra",
			value:    42,
			existing: map[string]interface{}{"result": "keep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &proxy.Response{Data: tt.existing}
			rc := NewResponseContext(resp)
			rc.WithData(tt.key, tt.value)

			assert.Equal(t, tt.value, resp.Data[tt.key])
		})
	}
}

func TestResponseContext_Response(t *testing.T) {
	t.Parallel()

	resp := &proxy.Response{IsComplete: true}
	rc := NewResponseContext(resp)
	assert.Equal(t, resp, rc.Response())
}

func TestNewRequestContext_WithURL(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("https://api.example.com/v1/users?page=1")
	req := &proxy.Request{
		Method: "GET",
		URL:    u,
		Path:   "/v1/users",
		Query:  u.Query(),
	}
	rc := NewRequestContext(req)
	assert.Equal(t, "GET", rc.Request().Method)
	assert.Equal(t, "/v1/users", rc.Request().Path)
}
