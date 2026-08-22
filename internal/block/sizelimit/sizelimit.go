// Package sizelimit implements a request body size limit block for the gateway.
// It rejects oversized payloads with 413 Payload Too Large using io.LimitReader.
package sizelimit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// SizeLimitBlock implements the Block interface for request body size limiting.
type SizeLimitBlock struct {
	config SizeLimitConfig
}

// NewSizeLimitBlock creates a new SizeLimitBlock with the given configuration.
func NewSizeLimitBlock(cfg SizeLimitConfig) (*SizeLimitBlock, error) {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating size limit block: %w", err)
	}

	return &SizeLimitBlock{config: cfg}, nil
}

// Name returns the block identifier.
func (b *SizeLimitBlock) Name() string {
	return "size-limit"
}

// Middleware returns a Lura proxy.Middleware that enforces request body size limits.
// Fast path: checks Content-Length header before reading body.
// Slow path: wraps body with io.LimitReader for streaming limit enforcement.
func (b *SizeLimitBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			maxBytes := b.config.MaxBytes

			// Fast path: check Content-Length header before reading body
			if clStr := http.Header(req.Headers).Get("Content-Length"); clStr != "" {
				cl, err := strconv.ParseInt(clStr, 10, 64)
				if err == nil && cl > maxBytes {
					return nil, domain.NewProblemError(
						http.StatusRequestEntityTooLarge,
						"payload_too_large",
						fmt.Sprintf("request body exceeds %d bytes", maxBytes),
					)
				}
			}

			// Wrap body with io.LimitReader if present.
			// The +1 allows detecting when the limit is actually exceeded.
			if req.Body != nil {
				req.Body = io.NopCloser(io.LimitReader(req.Body, maxBytes+1))
			}

			return nextProxy(ctx, req)
		}
	}
}
