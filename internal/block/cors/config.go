package cors

import "fmt"

// CORSConfig defines CORS behavior for a route or global scope.
type CORSConfig struct {
	// AllowedOrigins is the list of origins permitted to access the resource.
	// Defaults to empty (no origins allowed — restrictive default).
	AllowedOrigins []string

	// AllowedMethods is the list of HTTP methods allowed for CORS requests.
	// Defaults to GET, POST, HEAD.
	AllowedMethods []string

	// AllowedHeaders is the list of request headers allowed in CORS requests.
	// Defaults to Origin, Content-Type, Accept, Authorization.
	AllowedHeaders []string

	// AllowCredentials indicates whether the response to the request can include
	// credentials (cookies, HTTP auth, client-side SSL certificates).
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request
	// can be cached. Defaults to 300.
	MaxAge int

	// AllowAllOrigins must be explicitly set to true to allow "*" as an origin.
	// This is a safety gate — even if AllowedOrigins contains "*", it will be
	// ignored unless AllowAllOrigins is true.
	AllowAllOrigins bool
}

// Validate checks the CORS configuration for invalid combinations.
func (c *CORSConfig) Validate() error {
	if c.AllowAllOrigins && c.AllowCredentials {
		return fmt.Errorf("validating CORS config: wildcard origins with credentials is forbidden by the CORS specification")
	}
	return nil
}

// applyDefaults fills in zero-value fields with safe defaults.
func (c *CORSConfig) applyDefaults() {
	if len(c.AllowedMethods) == 0 {
		c.AllowedMethods = []string{"GET", "POST", "HEAD"}
	}
	if len(c.AllowedHeaders) == 0 {
		c.AllowedHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	}
	if c.MaxAge == 0 {
		c.MaxAge = 300
	}
}
