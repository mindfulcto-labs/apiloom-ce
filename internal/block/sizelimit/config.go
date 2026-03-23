package sizelimit

import "fmt"

// DefaultMaxBytes is the default maximum request body size (10 MB).
const DefaultMaxBytes int64 = 10 * 1024 * 1024

// SizeLimitConfig defines the maximum allowed request body size.
type SizeLimitConfig struct {
	// MaxBytes is the maximum number of bytes allowed in a request body.
	// Defaults to 10 MB (10 * 1024 * 1024).
	MaxBytes int64
}

// Validate checks that the configuration is valid.
func (c *SizeLimitConfig) Validate() error {
	if c.MaxBytes <= 0 {
		return fmt.Errorf("validating size limit config: MaxBytes must be > 0, got %d", c.MaxBytes)
	}
	return nil
}

// applyDefaults fills in zero-value fields with safe defaults.
func (c *SizeLimitConfig) applyDefaults() {
	if c.MaxBytes == 0 {
		c.MaxBytes = DefaultMaxBytes
	}
}
