package gateway

import (
	"github.com/luraproject/lura/v2/proxy"
)

// BlockChain composes an ordered slice of Blocks into a single Lura Middleware.
// Blocks execute in order: the first block in the chain wraps the outermost layer.
type BlockChain struct {
	blocks []Block
}

// NewBlockChain creates a BlockChain from the given blocks.
// An empty block chain passes requests through to the backend proxy unchanged.
func NewBlockChain(blocks ...Block) *BlockChain {
	return &BlockChain{blocks: blocks}
}

// Middleware returns a single Lura Middleware that composes all blocks in order.
// Each block wraps the next, forming an onion-layer chain around the backend proxy.
func (bc *BlockChain) Middleware() proxy.Middleware {
	if len(bc.blocks) == 0 {
		return func(next ...proxy.Proxy) proxy.Proxy {
			if len(next) > 0 {
				return next[0]
			}
			return proxy.NoopProxy
		}
	}

	return func(next ...proxy.Proxy) proxy.Proxy {
		// Start from the innermost backend proxy
		var current proxy.Proxy
		if len(next) > 0 {
			current = next[0]
		} else {
			current = proxy.NoopProxy
		}

		// Wrap from last to first so first block is outermost
		for i := len(bc.blocks) - 1; i >= 0; i-- {
			mw := bc.blocks[i].Middleware()
			current = mw(current)
		}

		return current
	}
}

// Len returns the number of blocks in the chain.
func (bc *BlockChain) Len() int {
	return len(bc.blocks)
}
