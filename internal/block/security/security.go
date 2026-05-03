// Package security provides the OWASP, DLP, and secret-scanning middleware block.
// This stub implementation satisfies the gateway.Block interface and compiles the
// benchmark suite (benchmarks/rps_test.go). Full implementation tracked in VEN-39.
package security

import (
	"context"

	"github.com/luraproject/lura/v2/proxy"
	"github.com/prometheus/client_golang/prometheus"
)

// OWASPRuleConfig holds per-rule toggles for the OWASP block.
type OWASPRuleConfig struct {
	InjectionEnabled      bool
	MassAssignmentEnabled bool
	AllowedFields         []string
	ExcessiveDataEnabled  bool
	MaxResponseBytes      int
	BrokenAuthEnabled     bool
	SSRFEnabled           bool
}

// Config is the top-level configuration for the security block.
type Config struct {
	OWASPEnabled      bool
	DLPEnabled        bool
	SecretScanEnabled bool
	DLPStrategy       string
	OWASPRules        OWASPRuleConfig
}

// Block implements gateway.Block for the security pipeline.
type Block struct {
	cfg Config
}

// New creates a security block. The prometheus.Registerer is accepted for future
// metrics instrumentation and is intentionally unused in this stub.
func New(cfg Config, _ prometheus.Registerer) (*Block, error) {
	return &Block{cfg: cfg}, nil
}

// Name returns the block identifier.
func (b *Block) Name() string { return "security" }

// Middleware returns a pass-through Lura proxy middleware.
// TODO(VEN-39): implement OWASP injection check, DLP redaction, and secret scanning.
func (b *Block) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}
		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			return nextProxy(ctx, req)
		}
	}
}
