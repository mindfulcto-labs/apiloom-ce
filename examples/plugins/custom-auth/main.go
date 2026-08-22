// Package main demonstrates a native Go plugin that validates custom token
// authentication. It checks for a token with a configurable prefix followed
// by base64-encoded data in either the X-Custom-Token or Authorization header.
package main

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/pkg/flex"
	"github.com/luraproject/lura/v2/proxy"
)

// CustomAuthConfig holds the configuration for the custom auth plugin.
type CustomAuthConfig struct {
	// TokenPrefix is the expected prefix before the base64 payload (default "flex-").
	TokenPrefix string
	// RequireBase64 requires the token payload after the prefix to be valid base64.
	RequireBase64 bool
}

// CustomAuthBlock validates custom token authentication on incoming requests.
type CustomAuthBlock struct {
	flex.BaseBlock
	config CustomAuthConfig
}

// NewCustomAuthBlock creates a CustomAuthBlock with the given configuration.
func NewCustomAuthBlock(cfg CustomAuthConfig) *CustomAuthBlock {
	if cfg.TokenPrefix == "" {
		cfg.TokenPrefix = "flex-"
	}
	return &CustomAuthBlock{
		BaseBlock: flex.BaseBlock{BlockName: "custom-auth"},
		config:    cfg,
	}
}

// Middleware returns a proxy middleware that validates the custom token.
func (b *CustomAuthBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) == 0 {
			return func(_ context.Context, _ *proxy.Request) (*proxy.Response, error) {
				return nil, nil
			}
		}
		backend := next[0]

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			token := extractToken(req)
			if token == "" {
				return nil, domain.NewProblemError(401, "missing-token",
					"request must include a token in X-Custom-Token or Authorization header")
			}

			if !strings.HasPrefix(token, b.config.TokenPrefix) {
				return nil, domain.NewProblemError(401, "invalid-token-prefix",
					"token does not have the required prefix")
			}

			if b.config.RequireBase64 {
				payload := strings.TrimPrefix(token, b.config.TokenPrefix)
				if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
					return nil, domain.NewProblemError(401, "invalid-token-encoding",
						"token payload is not valid base64")
				}
			}

			return backend(ctx, req)
		}
	}
}

// extractToken looks for a token in X-Custom-Token first, then Authorization.
func extractToken(req *proxy.Request) string {
	if vals, ok := req.Headers["X-Custom-Token"]; ok && len(vals) > 0 {
		return vals[0]
	}
	if vals, ok := req.Headers["Authorization"]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func main() {
	// In a real deployment, this plugin would be registered with the gateway.
	// This main() exists to make the example a valid standalone binary.
	_ = NewCustomAuthBlock(CustomAuthConfig{
		TokenPrefix:   "flex-",
		RequireBase64: true,
	})
}
