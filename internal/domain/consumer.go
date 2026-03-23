package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Consumer represents an API consumer with identity and metadata.
// Consumers own API keys and are attached to request context during authentication.
type Consumer struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Name           string          `json:"name"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// APIKey represents a hashed API key belonging to a consumer.
// The raw key is returned only once at creation time and never stored.
type APIKey struct {
	ID                uuid.UUID       `json:"id"`
	OrganizationID    uuid.UUID       `json:"organization_id"`
	ConsumerID        uuid.UUID       `json:"consumer_id"`
	KeyHash           string          `json:"-"`
	KeyPrefix         string          `json:"key_prefix"`
	IsActive          bool            `json:"is_active"`
	DeprecatedAt      *time.Time      `json:"deprecated_at,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`
	LastUsedAt        *time.Time      `json:"last_used_at,omitempty"`
	RateLimitOverride json.RawMessage `json:"rate_limit_override,omitempty"`
	Permissions       json.RawMessage `json:"permissions,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

// HasScope checks whether the key permits the given scope.
// Unrestricted keys (nil or empty permissions) allow all scopes.
// Supports wildcard matching: "*" in a scope segment matches any value.
func (k *APIKey) HasScope(scope string) bool {
	if !k.IsRestricted() {
		return true
	}

	var scopes []string
	if err := json.Unmarshal(k.Permissions, &scopes); err != nil {
		return false
	}

	reqParts := splitScope(scope)
	for _, s := range scopes {
		if matchScope(splitScope(s), reqParts) {
			return true
		}
	}
	return false
}

// IsRestricted returns true if the key has explicit scope restrictions.
func (k *APIKey) IsRestricted() bool {
	if len(k.Permissions) == 0 {
		return false
	}
	// Check for empty JSON array.
	var scopes []string
	if err := json.Unmarshal(k.Permissions, &scopes); err != nil {
		return false
	}
	return len(scopes) > 0
}

// splitScope splits a scope string on ":" into segments.
func splitScope(scope string) []string {
	parts := make([]string, 0, 3)
	start := 0
	for i := range scope {
		if scope[i] == ':' {
			parts = append(parts, scope[start:i])
			start = i + 1
		}
	}
	parts = append(parts, scope[start:])
	return parts
}

// matchScope compares a pattern (which may contain "*" wildcards) against a target.
func matchScope(pattern, target []string) bool {
	if len(pattern) != len(target) {
		return false
	}
	for i := range pattern {
		if pattern[i] != "*" && pattern[i] != target[i] {
			return false
		}
	}
	return true
}
