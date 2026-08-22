// Package main demonstrates a native Go plugin that modifies responses by
// adding headers and data fields. This is useful for injecting metadata like
// processing timestamps, version info, or trace IDs into API responses.
package main

import (
	"context"

	"github.com/mindfulcto-labs/apiloom-ce/pkg/flex"
	"github.com/luraproject/lura/v2/proxy"
)

// ResponseTransformConfig holds the configuration for the response transform plugin.
type ResponseTransformConfig struct {
	// AddHeaders maps header names to values that will be added to every response.
	AddHeaders map[string]string
	// AddFields maps field names to values that will be added to every response's data.
	AddFields map[string]interface{}
}

// ResponseTransformBlock modifies responses by adding configured headers and data fields.
type ResponseTransformBlock struct {
	flex.BaseBlock
	config ResponseTransformConfig
}

// NewResponseTransformBlock creates a ResponseTransformBlock with the given configuration.
func NewResponseTransformBlock(cfg ResponseTransformConfig) *ResponseTransformBlock {
	return &ResponseTransformBlock{
		BaseBlock: flex.BaseBlock{BlockName: "response-transform"},
		config:    cfg,
	}
}

// Middleware returns a proxy middleware that transforms responses.
func (b *ResponseTransformBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) == 0 {
			return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				return nil, nil
			}
		}
		backend := next[0]

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			resp, err := backend(ctx, req)
			if err != nil {
				return nil, err
			}
			if resp == nil {
				return nil, nil
			}

			// Add configured headers
			if len(b.config.AddHeaders) > 0 {
				if resp.Metadata.Headers == nil {
					resp.Metadata.Headers = make(map[string][]string)
				}
				for k, v := range b.config.AddHeaders {
					resp.Metadata.Headers[k] = []string{v}
				}
			}

			// Add configured data fields
			if len(b.config.AddFields) > 0 {
				if resp.Data == nil {
					resp.Data = make(map[string]interface{})
				}
				for k, v := range b.config.AddFields {
					resp.Data[k] = v
				}
			}

			return resp, nil
		}
	}
}

func main() {
	// In a real deployment, this plugin would be registered with the gateway.
	_ = NewResponseTransformBlock(ResponseTransformConfig{
		AddHeaders: map[string]string{
			"X-Transformed-By": "flex-gateway",
		},
		AddFields: map[string]interface{}{
			"transformed_at": "2026-01-01T00:00:00Z",
		},
	})
}
