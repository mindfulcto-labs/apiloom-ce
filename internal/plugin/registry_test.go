package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/pkg/flex"
)

// testBlock is a simple Block implementation for testing.
type testBlock struct {
	name string
}

func (b *testBlock) Name() string                  { return b.name }
func (b *testBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) > 0 {
			return next[0]
		}
		return proxy.NoopProxy
	}
}

// mockPluginStore implements PluginStore for unit testing.
type mockPluginStore struct {
	listFn          func(ctx context.Context, params ListPluginParams) (*PagedPlugins, error)
	createFn        func(ctx context.Context, rec *PluginRecord) (*PluginRecord, error)
	getFn           func(ctx context.Context, id uuid.UUID) (*PluginRecord, error)
	updateConfigFn  func(ctx context.Context, id uuid.UUID, config json.RawMessage) (*PluginRecord, error)
	updateEnabledFn func(ctx context.Context, id uuid.UUID, enabled bool) (*PluginRecord, error)
	updateOrderFn   func(ctx context.Context, id uuid.UUID, ordering int) (*PluginRecord, error)
	deleteFn        func(ctx context.Context, id uuid.UUID) error
}

func (m *mockPluginStore) Create(ctx context.Context, rec *PluginRecord) (*PluginRecord, error) {
	return m.createFn(ctx, rec)
}
func (m *mockPluginStore) Get(ctx context.Context, id uuid.UUID) (*PluginRecord, error) {
	return m.getFn(ctx, id)
}
func (m *mockPluginStore) List(ctx context.Context, params ListPluginParams) (*PagedPlugins, error) {
	return m.listFn(ctx, params)
}
func (m *mockPluginStore) UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) (*PluginRecord, error) {
	return m.updateConfigFn(ctx, id, config)
}
func (m *mockPluginStore) UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*PluginRecord, error) {
	return m.updateEnabledFn(ctx, id, enabled)
}
func (m *mockPluginStore) UpdateOrdering(ctx context.Context, id uuid.UUID, ordering int) (*PluginRecord, error) {
	return m.updateOrderFn(ctx, id, ordering)
}
func (m *mockPluginStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

func TestRegistry_RegisterAndLoad(t *testing.T) {
	t.Parallel()

	store := &mockPluginStore{
		listFn: func(_ context.Context, params ListPluginParams) (*PagedPlugins, error) {
			require.NotNil(t, params.IsEnabled)
			require.True(t, *params.IsEnabled)
			return &PagedPlugins{
				Data: []PluginRecord{
					{Name: "rate-limit", PluginType: "native", Config: json.RawMessage(`{"rps":100}`), Ordering: 20},
					{Name: "auth", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 10},
				},
			}, nil
		},
	}

	reg := NewRegistry(store)

	// Register factories.
	reg.Register("auth", func(cfg json.RawMessage) (flex.Block, error) {
		return &testBlock{name: "auth"}, nil
	})
	reg.Register("rate-limit", func(cfg json.RawMessage) (flex.Block, error) {
		return &testBlock{name: "rate-limit"}, nil
	})

	err := reg.LoadFromStore(context.Background())
	require.NoError(t, err)

	blocks := reg.Blocks()
	require.Len(t, blocks, 2)
	// Should be sorted by ordering: auth(10) before rate-limit(20).
	assert.Equal(t, "auth", blocks[0].Name())
	assert.Equal(t, "rate-limit", blocks[1].Name())
}

func TestRegistry_Reload(t *testing.T) {
	t.Parallel()

	callCount := 0
	store := &mockPluginStore{
		listFn: func(_ context.Context, _ ListPluginParams) (*PagedPlugins, error) {
			callCount++
			if callCount == 1 {
				return &PagedPlugins{
					Data: []PluginRecord{
						{Name: "plugin-a", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 1},
					},
				}, nil
			}
			// Second call: two plugins.
			return &PagedPlugins{
				Data: []PluginRecord{
					{Name: "plugin-a", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 1},
					{Name: "plugin-b", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 2},
				},
			}, nil
		},
	}

	reg := NewRegistry(store)
	reg.Register("plugin-a", func(_ json.RawMessage) (flex.Block, error) {
		return &testBlock{name: "plugin-a"}, nil
	})
	reg.Register("plugin-b", func(_ json.RawMessage) (flex.Block, error) {
		return &testBlock{name: "plugin-b"}, nil
	})

	// Initial load.
	err := reg.LoadFromStore(context.Background())
	require.NoError(t, err)
	assert.Len(t, reg.Blocks(), 1)

	// Reload picks up changes.
	err = reg.Reload(context.Background())
	require.NoError(t, err)
	assert.Len(t, reg.Blocks(), 2)
}

func TestRegistry_GRPCPlaceholder(t *testing.T) {
	t.Parallel()

	store := &mockPluginStore{
		listFn: func(_ context.Context, _ ListPluginParams) (*PagedPlugins, error) {
			addr := "localhost:50051"
			return &PagedPlugins{
				Data: []PluginRecord{
					{Name: "grpc-plugin", PluginType: "grpc", Config: json.RawMessage(`{}`), GRPCAddress: &addr, Ordering: 1},
				},
			}, nil
		},
	}

	reg := NewRegistry(store)
	err := reg.LoadFromStore(context.Background())
	require.NoError(t, err)

	blocks := reg.Blocks()
	require.Len(t, blocks, 1)
	assert.Equal(t, "grpc-plugin", blocks[0].Name())
	// Middleware should not be nil (placeholder pass-through).
	assert.NotNil(t, blocks[0].Middleware())
}

func TestRegistry_MissingFactory(t *testing.T) {
	t.Parallel()

	store := &mockPluginStore{
		listFn: func(_ context.Context, _ ListPluginParams) (*PagedPlugins, error) {
			return &PagedPlugins{
				Data: []PluginRecord{
					{Name: "unknown-plugin", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 1},
				},
			}, nil
		},
	}

	reg := NewRegistry(store)
	err := reg.LoadFromStore(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no factory registered")
}

func TestRegistry_BlocksReturnsCopy(t *testing.T) {
	t.Parallel()

	store := &mockPluginStore{
		listFn: func(_ context.Context, _ ListPluginParams) (*PagedPlugins, error) {
			return &PagedPlugins{
				Data: []PluginRecord{
					{Name: "plugin-a", PluginType: "native", Config: json.RawMessage(`{}`), Ordering: 1},
				},
			}, nil
		},
	}

	reg := NewRegistry(store)
	reg.Register("plugin-a", func(_ json.RawMessage) (flex.Block, error) {
		return &testBlock{name: "plugin-a"}, nil
	})

	err := reg.LoadFromStore(context.Background())
	require.NoError(t, err)

	blocks1 := reg.Blocks()
	blocks2 := reg.Blocks()
	// Modifying one slice should not affect the other.
	blocks1[0] = nil
	assert.NotNil(t, blocks2[0])
}
