package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TrustLevel represents the trust level assigned to an AI agent.
type TrustLevel string

const (
	TrustUntrusted TrustLevel = "untrusted"
	TrustBasic     TrustLevel = "basic"
	TrustElevated  TrustLevel = "elevated"
	TrustAdmin     TrustLevel = "admin"
)

// validTrustLevels enumerates all valid trust levels for validation.
var validTrustLevels = map[TrustLevel]bool{
	TrustUntrusted: true,
	TrustBasic:     true,
	TrustElevated:  true,
	TrustAdmin:     true,
}

// ValidTrustLevels returns the set of valid trust levels for external validation.
func ValidTrustLevels() map[TrustLevel]bool {
	return validTrustLevels
}

// AgentStatus represents the lifecycle status of an AI agent.
type AgentStatus string

const (
	AgentActive    AgentStatus = "active"
	AgentSuspended AgentStatus = "suspended"
	AgentRevoked   AgentStatus = "revoked"
)

// AutonomyLevel controls how much autonomous action an agent can take.
// low = human approval for sensitive actions, medium = notify on sensitive, high = fully autonomous.
type AutonomyLevel string

const (
	AutonomyLow    AutonomyLevel = "low"
	AutonomyMedium AutonomyLevel = "medium"
	AutonomyHigh   AutonomyLevel = "high"
)

// validAutonomyLevels enumerates all valid autonomy levels for validation.
var validAutonomyLevels = map[AutonomyLevel]bool{
	AutonomyLow:    true,
	AutonomyMedium: true,
	AutonomyHigh:   true,
}

// defaultMaxCallDepth is used when MaxCallDepth is not specified.
const defaultAgentMaxCallDepth = 10

// Agent represents an AI agent registered with FlexGateway.
// Each agent has a unique identity with trust level, capabilities, and lifecycle status.
type Agent struct {
	ID            uuid.UUID       `json:"id"`
	OrgID         uuid.UUID       `json:"org_id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	OwnerID       uuid.UUID       `json:"owner_id"`
	Capabilities  []string        `json:"capabilities"`
	TrustLevel    TrustLevel      `json:"trust_level"`
	Status        AgentStatus     `json:"status"`
	APIKeyHash    string          `json:"-"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	// AGT-01: Behavioral governance fields.
	AutonomyLevel AutonomyLevel   `json:"autonomy_level"`
	MaxCallDepth  int             `json:"max_call_depth"`
	AllowedTools  []string        `json:"allowed_tools,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Validate checks that the agent has valid required fields.
// It also applies defaults: AutonomyLevel defaults to "low", MaxCallDepth defaults to 10.
func (a *Agent) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if len(a.Name) > 255 {
		return fmt.Errorf("agent name must be 255 characters or fewer")
	}
	if !validTrustLevels[a.TrustLevel] {
		return fmt.Errorf("invalid trust level %q: must be one of untrusted, basic, elevated, admin", a.TrustLevel)
	}
	if a.Capabilities == nil {
		return fmt.Errorf("capabilities must not be nil")
	}

	// AutonomyLevel: default to "low" (safest) if empty.
	if a.AutonomyLevel == "" {
		a.AutonomyLevel = AutonomyLow
	} else if !validAutonomyLevels[a.AutonomyLevel] {
		return fmt.Errorf("invalid autonomy level %q: must be one of low, medium, high", a.AutonomyLevel)
	}

	// MaxCallDepth: default to 10 if zero, reject if > 100.
	if a.MaxCallDepth == 0 {
		a.MaxCallDepth = defaultAgentMaxCallDepth
	} else if a.MaxCallDepth > 100 {
		return fmt.Errorf("max_call_depth must not exceed 100, got %d", a.MaxCallDepth)
	}

	// AllowedTools: nil is valid (no restriction). No validation needed.

	return nil
}
