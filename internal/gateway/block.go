// Package gateway provides the HTTP reverse proxy server and block middleware chain.
package gateway

import (
	"github.com/luraproject/lura/v2/proxy"
)

// Block is the extension point for all middleware in the gateway pipeline.
// Every feature beyond core routing (auth, rate limiting, AI proxy, CORS, etc.)
// implements Block. Max 3 methods per CLAUDE.md rule 4.
type Block interface {
	// Name returns a unique identifier for the block (e.g., "auth-jwt", "rate-limit").
	Name() string

	// Middleware returns a Lura-compatible proxy middleware that wraps the next proxy
	// in the chain. The middleware can inspect/modify the request before forwarding
	// and inspect/modify the response after receiving it.
	Middleware() proxy.Middleware
}
