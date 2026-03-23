package flex

import (
	"context"

	"github.com/luraproject/lura/v2/proxy"
)

// BaseBlock provides no-op defaults for the Block interface. Embed it in your
// plugin struct so you only need to override the methods you care about.
//
//	type MyPlugin struct {
//	    flex.BaseBlock
//	}
//
//	func NewMyPlugin() *MyPlugin {
//	    return &MyPlugin{BaseBlock: flex.BaseBlock{BlockName: "my-plugin"}}
//	}
type BaseBlock struct {
	// BlockName is the plugin's unique identifier. Override via struct literal
	// or set after construction.
	BlockName string
}

// Name returns the block's identifier. Returns empty string if BlockName is not set.
func (b *BaseBlock) Name() string {
	return b.BlockName
}

// Middleware returns a pass-through middleware that forwards requests to the
// next proxy unchanged. Override this method to add plugin behavior.
func (b *BaseBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) > 0 {
			return next[0]
		}
		return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
			return nil, nil
		}
	}
}
