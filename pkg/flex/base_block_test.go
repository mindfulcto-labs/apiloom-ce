package flex

import (
	"context"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseBlock_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blockName string
		want      string
	}{
		{name: "empty name", blockName: "", want: ""},
		{name: "custom name", blockName: "rate-limit", want: "rate-limit"},
		{name: "hyphenated", blockName: "auth-jwt", want: "auth-jwt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := BaseBlock{BlockName: tt.blockName}
			assert.Equal(t, tt.want, b.Name())
		})
	}
}

func TestBaseBlock_Middleware_Passthrough(t *testing.T) {
	t.Parallel()

	b := BaseBlock{}
	mw := b.Middleware()
	require.NotNil(t, mw)

	called := false
	backend := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		called = true
		return &proxy.Response{Data: map[string]interface{}{"passed": true}, IsComplete: true}, nil
	}

	p := mw(backend)
	resp, err := p(context.Background(), &proxy.Request{Method: "GET"})
	require.NoError(t, err)
	assert.True(t, called, "backend proxy must be called")
	assert.True(t, resp.IsComplete)
	assert.Equal(t, true, resp.Data["passed"])
}

func TestBaseBlock_Middleware_NoBackend(t *testing.T) {
	t.Parallel()

	b := BaseBlock{}
	mw := b.Middleware()

	// When called with no next proxies, should use NoopProxy behavior
	p := mw()
	resp, err := p(context.Background(), &proxy.Request{})
	require.NoError(t, err)
	require.Nil(t, resp)
}
