package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Environment represents a deployment environment (e.g., dev, staging, prod)
// scoped to an organization.
type Environment struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description"`
	IsProduction   bool      `json:"is_production"`
	SortOrder      int       `json:"sort_order"`
	RouteCount     int       `json:"route_count,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PromotionResult holds the counts of resources created/updated during
// environment promotion.
type PromotionResult struct {
	RoutesCreated   int `json:"routes_created"`
	RoutesUpdated   int `json:"routes_updated"`
	BackendsCreated int `json:"backends_created"`
	BackendsUpdated int `json:"backends_updated"`
}

// MaxEnvironmentsPerOrg is the limit on how many environments a single org can have.
const MaxEnvironmentsPerOrg = 10

var slugRegexp = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify derives a URL-safe slug from a name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRegexp.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Validate checks that the environment fields satisfy business rules.
func (e *Environment) Validate() error {
	name := strings.TrimSpace(e.Name)
	if len(name) < 2 || len(name) > 50 {
		return fmt.Errorf("environment name must be 2-50 characters: %w", ErrInvalidConfig)
	}
	e.Name = name

	if e.Slug == "" {
		e.Slug = Slugify(e.Name)
	}
	if e.Slug == "" {
		return fmt.Errorf("environment slug cannot be empty: %w", ErrInvalidConfig)
	}

	return nil
}

// DefaultEnvironments returns the three default environments seeded for a new org.
func DefaultEnvironments(orgID uuid.UUID) []Environment {
	now := time.Now().UTC()
	return []Environment{
		{
			ID: uuid.New(), OrganizationID: orgID,
			Name: "Development", Slug: "development", Description: "Development environment",
			IsProduction: false, SortOrder: 0,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: uuid.New(), OrganizationID: orgID,
			Name: "Staging", Slug: "staging", Description: "Staging environment",
			IsProduction: false, SortOrder: 1,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: uuid.New(), OrganizationID: orgID,
			Name: "Production", Slug: "production", Description: "Production environment",
			IsProduction: true, SortOrder: 2,
			CreatedAt: now, UpdatedAt: now,
		},
	}
}
