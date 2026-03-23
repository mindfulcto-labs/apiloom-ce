// Package jwt provides JWT validation as a Block middleware.
package jwt

import (
	"errors"
	"time"
)

// JWTConfig holds the configuration for JWT validation.
type JWTConfig struct {
	// JWKSEndpoint is the URL to fetch the JSON Web Key Set from.
	JWKSEndpoint string

	// AllowedIssuers is the set of acceptable token issuers (iss claim).
	AllowedIssuers []string

	// AllowedAudiences is the set of acceptable token audiences (aud claim).
	AllowedAudiences []string

	// AllowedAlgorithms restricts which signing algorithms are accepted.
	// Defaults to ["RS256"] if empty.
	AllowedAlgorithms []string

	// ClaimMap maps token claim names to upstream request header names.
	// Only claims listed here are forwarded; all others are dropped.
	// Example: {"sub": "X-User-Sub", "email": "X-User-Email"}
	ClaimMap map[string]string

	// RefreshInterval is how often the JWKS cache refreshes in the background.
	// Defaults to 15 minutes if zero.
	RefreshInterval time.Duration
}

// Validate checks that all required configuration fields are present.
func (c *JWTConfig) Validate() error {
	if c.JWKSEndpoint == "" {
		return errors.New("validating jwt config: jwks_endpoint is required")
	}
	if len(c.AllowedIssuers) == 0 {
		return errors.New("validating jwt config: allowed_issuers is required")
	}
	if len(c.AllowedAudiences) == 0 {
		return errors.New("validating jwt config: allowed_audiences is required")
	}
	return nil
}

// defaults fills in zero-value fields with sensible defaults.
func (c *JWTConfig) defaults() {
	if len(c.AllowedAlgorithms) == 0 {
		c.AllowedAlgorithms = []string{"RS256"}
	}
	if c.RefreshInterval == 0 {
		c.RefreshInterval = 15 * time.Minute
	}
	if c.ClaimMap == nil {
		c.ClaimMap = make(map[string]string)
	}
}
