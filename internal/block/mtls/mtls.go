// Package mtls implements a mutual TLS block for the gateway proxy pipeline.
// It configures client certificates for upstream connections and enforces
// TLS 1.2 minimum. Certificates are loaded via GetClientCertificate callback
// to support cert-manager rotation.
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/luraproject/lura/v2/proxy"
)

// tlsConfigKey is the unexported context key for storing *tls.Config.
type tlsConfigKey struct{}

// GetTLSConfig retrieves the *tls.Config from the context, if set by the mTLS block.
// Returns nil if no mTLS config is present.
func GetTLSConfig(ctx context.Context) *tls.Config {
	cfg, _ := ctx.Value(tlsConfigKey{}).(*tls.Config)
	return cfg
}

// MTLSBlock implements the Block interface for mutual TLS to upstream services.
type MTLSBlock struct {
	config    MTLSConfig
	tlsConfig *tls.Config
}

// NewMTLSBlock creates a new MTLSBlock with the given configuration.
// It validates the config and builds a tls.Config with GetClientCertificate callback.
func NewMTLSBlock(cfg MTLSConfig) (*MTLSBlock, error) {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("creating mTLS block: %w", err)
	}

	// Verify the cert/key files are readable at startup.
	if _, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}

	tlsCfg := &tls.Config{
		MinVersion: cfg.MinTLSVersion,
		// GetClientCertificate loads cert from disk on each connection for rotation support.
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("loading client certificate for mTLS: %w", err)
			}
			return &cert, nil
		},
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	if len(cfg.CipherSuites) > 0 {
		tlsCfg.CipherSuites = cfg.CipherSuites
	}

	// Load custom CA bundle if provided.
	if cfg.CAFile != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file %q: %w", cfg.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parsing CA file %q: no valid certificates found", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	return &MTLSBlock{
		config:    cfg,
		tlsConfig: tlsCfg,
	}, nil
}

// Name returns the block identifier.
func (b *MTLSBlock) Name() string {
	return "mtls"
}

// Middleware returns a Lura proxy.Middleware that stores the tls.Config in the
// request context. Downstream code (e.g., buildEndpointHandler in server.go)
// retrieves it via GetTLSConfig to configure the HTTP transport for upstream calls.
func (b *MTLSBlock) Middleware() proxy.Middleware {
	return func(next ...proxy.Proxy) proxy.Proxy {
		var nextProxy proxy.Proxy
		if len(next) > 0 {
			nextProxy = next[0]
		} else {
			nextProxy = proxy.NoopProxy
		}

		return func(ctx context.Context, req *proxy.Request) (*proxy.Response, error) {
			ctx = context.WithValue(ctx, tlsConfigKey{}, b.tlsConfig)
			return nextProxy(ctx, req)
		}
	}
}

// TLSConfig returns the constructed tls.Config for direct use by the server
// when building HTTP transports for mTLS-enabled backends.
func (b *MTLSBlock) TLSConfig() *tls.Config {
	return b.tlsConfig
}
