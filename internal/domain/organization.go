package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Role constants for organization members.
const (
	RolePlatformAdmin = "platform_admin"
	RoleOrgAdmin      = "org_admin"
	RoleDeveloper     = "developer"
	RoleViewer        = "viewer"
)

// validRoles is the set of allowed member roles.
var validRoles = map[string]bool{
	RolePlatformAdmin: true,
	RoleOrgAdmin:      true,
	RoleDeveloper:     true,
	RoleViewer:        true,
}

// Organization represents a tenant in the multi-tenancy model.
// All configuration entities (routes, backends, consumers, etc.) belong to an organization.
type Organization struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	BillingEmail string          `json:"billing_email"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Team represents a group within an organization.
type Team struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	Name           string          `json:"name"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Member represents a user's membership in an organization, optionally scoped to a team.
type Member struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	TeamID         *uuid.UUID `json:"team_id,omitempty"`
	UserID         string     `json:"user_id"`
	Role           string     `json:"role"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ValidateRole returns an error if the given role is not one of the allowed roles.
func ValidateRole(role string) error {
	if !validRoles[role] {
		return fmt.Errorf("invalid role %q: must be one of platform_admin, org_admin, developer, viewer", role)
	}
	return nil
}
