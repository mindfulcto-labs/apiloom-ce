package sizelimit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

func TestSizeLimitConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  SizeLimitConfig
		wantErr bool
	}{
		{
			name:    "zero MaxBytes after defaults is valid",
			config:  SizeLimitConfig{},
			wantErr: false, // applyDefaults sets it to 10MB before validate in NewSizeLimitBlock
		},
		{
			name:    "positive MaxBytes is valid",
			config:  SizeLimitConfig{MaxBytes: 1024},
			wantErr: false,
		},
		{
			name:    "negative MaxBytes is invalid",
			config:  SizeLimitConfig{MaxBytes: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := tt.config
			cfg.applyDefaults()
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSizeLimitBlock_Name(t *testing.T) {
	t.Parallel()
	block, err := NewSizeLimitBlock(SizeLimitConfig{})
	require.NoError(t, err)
	assert.Equal(t, "size-limit", block.Name())
}

func TestSizeLimitBlock_DefaultMaxBytes(t *testing.T) {
	t.Parallel()
	block, err := NewSizeLimitBlock(SizeLimitConfig{})
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxBytes, block.config.MaxBytes)
}

func TestSizeLimitBlock_ContentLengthOverLimit(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{MaxBytes: 100})
	require.NoError(t, err)

	nextCalled := false
	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		nextCalled = true
		return &proxy.Response{IsComplete: true}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	req := &proxy.Request{
		Method:  http.MethodPost,
		Headers: map[string][]string{"Content-Length": {"200"}},
		Body:    io.NopCloser(bytes.NewReader(make([]byte, 200))),
	}

	resp, err := p(context.Background(), req)

	// Should return 413 ProblemError
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.False(t, nextCalled, "should reject before calling next proxy")

	var pe *domain.ProblemError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, http.StatusRequestEntityTooLarge, pe.StatusCode())
}

func TestSizeLimitBlock_NoContentLengthBodyOverLimit(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{MaxBytes: 100})
	require.NoError(t, err)

	var capturedBody []byte
	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		// The body should be wrapped with LimitReader
		// Reading maxBytes+1 should hit the limit
		if req.Body != nil {
			var readErr error
			capturedBody, readErr = io.ReadAll(req.Body)
			if readErr != nil {
				return nil, readErr
			}
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

	// Body of 200 bytes but no Content-Length header
	body := make([]byte, 200)
	for i := range body {
		body[i] = 'a'
	}

	req := &proxy.Request{
		Method:  http.MethodPost,
		Headers: map[string][]string{},
		Body:    io.NopCloser(bytes.NewReader(body)),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// LimitReader should have capped the body at maxBytes+1 bytes
	assert.LessOrEqual(t, int64(len(capturedBody)), int64(101))
}

func TestSizeLimitBlock_BodyUnderLimit(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{MaxBytes: 1024})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			return nil, readErr
		}
		return &proxy.Response{
			IsComplete: true,
			Data:       map[string]interface{}{"body_len": len(body)},
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	body := make([]byte, 100)
	req := &proxy.Request{
		Method:  http.MethodPost,
		Headers: map[string][]string{"Content-Length": {"100"}},
		Body:    io.NopCloser(bytes.NewReader(body)),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 100, resp.Data["body_len"])
}

func TestSizeLimitBlock_NoBody(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{MaxBytes: 1024})
	require.NoError(t, err)

	nextCalled := false
	nextProxy := func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
		nextCalled = true
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
		Headers: map[string][]string{},
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, nextCalled)
}

func TestSizeLimitBlock_ExactLimit(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{MaxBytes: 100})
	require.NoError(t, err)

	nextProxy := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			return nil, readErr
		}
		return &proxy.Response{
			IsComplete: true,
			Data:       map[string]interface{}{"body_len": len(body)},
			Metadata: proxy.Metadata{
				StatusCode: 200,
				Headers:    http.Header{},
			},
		}, nil
	}

	mw := block.Middleware()
	p := mw(nextProxy)

	body := make([]byte, 100)
	req := &proxy.Request{
		Method:  http.MethodPost,
		Headers: map[string][]string{"Content-Length": {"100"}},
		Body:    io.NopCloser(bytes.NewReader(body)),
	}

	resp, err := p(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Exact limit should pass through
	assert.Equal(t, 100, resp.Data["body_len"])
}

func TestSizeLimitBlock_ImplementsBlockInterface(t *testing.T) {
	t.Parallel()

	block, err := NewSizeLimitBlock(SizeLimitConfig{})
	require.NoError(t, err)

	var _ interface {
		Name() string
		Middleware() proxy.Middleware
	} = block
}
