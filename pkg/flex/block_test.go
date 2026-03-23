package flex

import (
	"context"
	"testing"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pt   PluginType
		want string
	}{
		{name: "native", pt: PluginTypeNative, want: "native"},
		{name: "grpc", pt: PluginTypeGRPC, want: "grpc"},
		{name: "unknown", pt: PluginType(99), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.pt.String())
		})
	}
}

func TestPluginDescriptor_Fields(t *testing.T) {
	t.Parallel()

	d := PluginDescriptor{
		Name:         "my-plugin",
		Version:      "1.0.0",
		Type:         PluginTypeNative,
		ConfigSchema: []byte(`{"type":"object"}`),
	}
	assert.Equal(t, "my-plugin", d.Name)
	assert.Equal(t, "1.0.0", d.Version)
	assert.Equal(t, PluginTypeNative, d.Type)
	assert.JSONEq(t, `{"type":"object"}`, string(d.ConfigSchema))
}

// TestBlockInterface_Compliance verifies that a struct embedding BaseBlock
// satisfies the Block interface at compile time.
func TestBlockInterface_Compliance(t *testing.T) {
	t.Parallel()

	// This is a compile-time check — if this test compiles, BaseBlock satisfies Block.
	var _ Block = &struct{ BaseBlock }{}
}

// TestBlockInterface_CustomImpl verifies a custom struct implementing Block directly.
func TestBlockInterface_CustomImpl(t *testing.T) {
	t.Parallel()

	type customBlock struct {
		BaseBlock
	}

	b := &customBlock{}
	b.BlockName = "custom"

	// Must satisfy Block interface
	var block Block = b
	assert.Equal(t, "custom", block.Name())

	// Middleware must be non-nil
	mw := block.Middleware()
	require.NotNil(t, mw)

	// Pass-through: calling the middleware with a backend should return backend result
	backend := func(_ context.Context, req *proxy.Request) (*proxy.Response, error) {
		return &proxy.Response{Data: map[string]interface{}{"ok": true}, IsComplete: true}, nil
	}
	p := mw(backend)
	resp, err := p(context.Background(), &proxy.Request{})
	require.NoError(t, err)
	assert.True(t, resp.IsComplete)
	assert.Equal(t, true, resp.Data["ok"])
}
