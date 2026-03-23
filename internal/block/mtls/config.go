package mtls

import (
	"crypto/tls"
	"fmt"
	"log/slog"
)

// MTLSConfig defines mutual TLS configuration for upstream connections.
type MTLSConfig struct {
	// CertFile is the path to the client certificate PEM file.
	CertFile string

	// KeyFile is the path to the client private key PEM file.
	KeyFile string

	// CAFile is the optional path to the CA certificate bundle PEM file for
	// verifying upstream server certificates. If empty, the system CA pool is used.
	CAFile string

	// MinTLSVersion is the minimum TLS version to negotiate. Must be >= tls.VersionTLS12.
	// Defaults to tls.VersionTLS12.
	MinTLSVersion uint16

	// CipherSuites is an optional list of cipher suites to use. If empty, Go's
	// default cipher suite selection is used.
	CipherSuites []uint16

	// InsecureSkipVerify disables server certificate verification. MUST be false
	// in production per security.md. Only for testing.
	InsecureSkipVerify bool
}

// Validate checks that the mTLS configuration is valid.
func (c *MTLSConfig) Validate() error {
	if c.CertFile == "" {
		return fmt.Errorf("validating mTLS config: CertFile must not be empty")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("validating mTLS config: KeyFile must not be empty")
	}
	if c.MinTLSVersion != 0 && c.MinTLSVersion < tls.VersionTLS12 {
		return fmt.Errorf("validating mTLS config: MinTLSVersion must be >= TLS 1.2 (0x%04x), got 0x%04x", tls.VersionTLS12, c.MinTLSVersion)
	}
	if c.InsecureSkipVerify {
		slog.Warn("mTLS InsecureSkipVerify is true — this must NOT be used in production")
	}
	return nil
}

// applyDefaults fills in zero-value fields with safe defaults.
func (c *MTLSConfig) applyDefaults() {
	if c.MinTLSVersion == 0 {
		c.MinTLSVersion = tls.VersionTLS12
	}
}
