// Package domain contains domain types and sentinel errors shared across FlexGateway.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// GatewayConfig represents the full gateway configuration assembled from
// all active routes and backends.
type GatewayConfig struct {
	Version  int             `json:"version"`
	Routes   []RouteConfig   `json:"routes"`
	Backends []BackendConfig `json:"backends"`
	LoadedAt time.Time       `json:"loaded_at"`
}

// RouteConfig represents a single route entry in the gateway configuration.
type RouteConfig struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Name           string          `json:"name"`
	Version   int             `json:"version"`
	Config    json.RawMessage `json:"config"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Checksum  string          `json:"checksum"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// BackendConfig represents a single backend entry in the gateway configuration.
type BackendConfig struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Name           string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Relationship represents a directed relationship between two configuration entities.
// This enables graph-like traversal of config relationships, preparing for
// Apache AGE migration in Phase 2.
type Relationship struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	SourceType     string          `json:"source_type"`
	SourceID     uuid.UUID       `json:"source_id"`
	Relationship string          `json:"relationship"`
	TargetType   string          `json:"target_type"`
	TargetID     uuid.UUID       `json:"target_id"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// ComputeChecksum returns a hex-encoded SHA-256 hash of the given JSON config.
// Used for change detection on route configurations.
func ComputeChecksum(config json.RawMessage) string {
	h := sha256.Sum256(config)
	return hex.EncodeToString(h[:])
}
