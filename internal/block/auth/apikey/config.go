package apikey

import (
	"log/slog"
	"time"
)

// APIKeyConfig holds configuration for the API key authentication block.
type APIKeyConfig struct {
	// HeaderName is the HTTP header to read the API key from. Defaults to "X-API-Key".
	HeaderName string

	// AllowQueryParam enables reading the API key from the ?api_key= query parameter.
	AllowQueryParam bool

	// CacheTTL is the duration to cache resolved consumers. Defaults to 60s.
	CacheTTL time.Duration

	// Logger is the structured logger. If nil, slog.Default() is used.
	Logger *slog.Logger
}

// Validate applies defaults and validates the configuration.
func (c *APIKeyConfig) Validate() {
	if c.HeaderName == "" {
		c.HeaderName = "X-API-Key"
	}
	if c.CacheTTL == 0 {
		c.CacheTTL = 60 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}
