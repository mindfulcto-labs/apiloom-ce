package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/pkg/flex"
)

// BlockFactory creates a Block instance from plugin configuration JSON.
type BlockFactory func(config json.RawMessage) (flex.Block, error)

// Registry manages plugin lifecycle: registration of native plugin factories,
// loading enabled plugins from store, and producing ordered Block slices.
type Registry struct {
	factories map[string]BlockFactory
	store     PluginStore
	blocks    []flex.Block
	mu        sync.RWMutex
}

// NewRegistry creates a Registry backed by the given store.
func NewRegistry(store PluginStore) *Registry {
	return &Registry{
		factories: make(map[string]BlockFactory),
		store:     store,
	}
}

// Register adds a native plugin factory by name. This must be called
// before LoadFromStore to make native plugins available for instantiation.
func (r *Registry) Register(name string, factory BlockFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// LoadFromStore reads enabled plugins from the store, instantiates Block
// instances via registered factories (for native type) or placeholder (for gRPC),
// and sorts them by ordering.
func (r *Registry) LoadFromStore(ctx context.Context) error {
	enabled := true
	result, err := r.store.List(ctx, ListPluginParams{
		Limit:     100,
		IsEnabled: &enabled,
	})
	if err != nil {
		return fmt.Errorf("listing enabled plugins: %w", err)
	}

	// Sort by ordering.
	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Ordering < result.Data[j].Ordering
	})

	blocks := make([]flex.Block, 0, len(result.Data))
	for _, rec := range result.Data {
		block, err := r.instantiate(rec)
		if err != nil {
			return fmt.Errorf("instantiating plugin %s: %w", rec.Name, err)
		}
		blocks = append(blocks, block)
	}

	r.mu.Lock()
	r.blocks = blocks
	r.mu.Unlock()

	return nil
}

// Blocks returns the current ordered slice of active Block instances.
func (r *Registry) Blocks() []flex.Block {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]flex.Block, len(r.blocks))
	copy(out, r.blocks)
	return out
}

// Reload re-reads the store and rebuilds the block list.
func (r *Registry) Reload(ctx context.Context) error {
	return r.LoadFromStore(ctx)
}

// instantiate creates a Block from a PluginRecord using the appropriate factory.
func (r *Registry) instantiate(rec PluginRecord) (flex.Block, error) {
	switch rec.PluginType {
	case "native":
		factory, ok := r.factories[rec.Name]
		if !ok {
			return nil, fmt.Errorf("no factory registered for native plugin %q", rec.Name)
		}
		return factory(rec.Config)
	case "grpc":
		// gRPC plugins will be handled by the gRPC plugin host in a later plan.
		// For now, return a placeholder block.
		return &grpcPlaceholderBlock{name: rec.Name}, nil
	default:
		return nil, fmt.Errorf("unknown plugin type %q", rec.PluginType)
	}
}

// grpcPlaceholderBlock is a placeholder for gRPC plugins until the gRPC host is implemented.
type grpcPlaceholderBlock struct {
	name string
}

func (b *grpcPlaceholderBlock) Name() string {
	return b.name
}

func (b *grpcPlaceholderBlock) Middleware() proxy.Middleware {
	// No-op pass-through until the gRPC plugin host is implemented.
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) > 0 {
			return next[0]
		}
		return proxy.NoopProxy
	}
}
