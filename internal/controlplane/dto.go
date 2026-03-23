package controlplane

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// ListParams holds cursor-based pagination parameters.
type ListParams struct {
	Cursor string
	Limit  int
}

// EffectiveLimit returns the limit capped between 1 and maxLimit, defaulting to 20.
func (lp ListParams) EffectiveLimit() int {
	if lp.Limit <= 0 {
		return defaultLimit
	}
	if lp.Limit > maxLimit {
		return maxLimit
	}
	return lp.Limit
}

// ParseListParams extracts cursor and limit from query parameters.
func ParseListParams(q url.Values) ListParams {
	lp := ListParams{
		Cursor: q.Get("cursor"),
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			lp.Limit = n
		}
	}
	return lp
}

// PagedResponse wraps a data slice with cursor pagination metadata.
type PagedResponse[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// --- Route DTOs ---

// CreateRouteRequest is the DTO for creating a route.
type CreateRouteRequest struct {
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields and constraints.
func (r CreateRouteRequest) Validate() error {
	if err := validateName(r.Name, 3, 100); err != nil {
		return err
	}
	if err := validateJSONRequired(r.Config, "config"); err != nil {
		return err
	}
	return nil
}

// UpdateRouteRequest is the DTO for updating a route.
type UpdateRouteRequest struct {
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields.
func (r UpdateRouteRequest) Validate() error {
	if err := validateName(r.Name, 3, 100); err != nil {
		return err
	}
	if err := validateJSONRequired(r.Config, "config"); err != nil {
		return err
	}
	return nil
}

// --- Backend DTOs ---

// CreateBackendRequest is the DTO for creating a backend.
type CreateBackendRequest struct {
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields and constraints.
func (r CreateBackendRequest) Validate() error {
	if err := validateName(r.Name, 3, 100); err != nil {
		return err
	}
	if err := validateJSONRequired(r.Config, "config"); err != nil {
		return err
	}
	return nil
}

// UpdateBackendRequest is the DTO for updating a backend.
type UpdateBackendRequest struct {
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields.
func (r UpdateBackendRequest) Validate() error {
	if err := validateName(r.Name, 3, 100); err != nil {
		return err
	}
	if err := validateJSONRequired(r.Config, "config"); err != nil {
		return err
	}
	return nil
}

// --- Consumer DTOs ---

// CreateConsumerRequest is the DTO for creating a consumer.
type CreateConsumerRequest struct {
	Name     string          `json:"name"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields.
func (r CreateConsumerRequest) Validate() error {
	return validateName(r.Name, 3, 100)
}

// UpdateConsumerRequest is the DTO for updating a consumer.
type UpdateConsumerRequest struct {
	Name     string          `json:"name"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Validate checks required fields.
func (r UpdateConsumerRequest) Validate() error {
	return validateName(r.Name, 3, 100)
}

// --- API Key DTOs ---

// CreateAPIKeyRequest is the DTO for creating an API key with optional permissions.
type CreateAPIKeyRequest struct {
	Permissions json.RawMessage `json:"permissions,omitempty"`
}

// Validate checks that permissions, if provided, is a valid JSON array of scope strings
// matching the pattern api:{id}:{read|write|*}.
func (r CreateAPIKeyRequest) Validate() error {
	if len(r.Permissions) == 0 {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal(r.Permissions, &scopes); err != nil {
		return fmt.Errorf("permissions must be a JSON array of strings")
	}
	for _, s := range scopes {
		if err := validateScope(s); err != nil {
			return err
		}
	}
	return nil
}

// validateScope checks that a scope matches the pattern api:{id}:{read|write|*}.
func validateScope(scope string) error {
	parts := splitScopeStr(scope)
	if len(parts) != 3 {
		return fmt.Errorf("scope %q must have format api:{id}:{read|write|*}", scope)
	}
	if parts[0] != "api" {
		return fmt.Errorf("scope %q must start with 'api'", scope)
	}
	if parts[1] == "" {
		return fmt.Errorf("scope %q has empty API ID segment", scope)
	}
	switch parts[2] {
	case "read", "write", "*":
		return nil
	default:
		return fmt.Errorf("scope %q action must be read, write, or *", scope)
	}
}

// splitScopeStr splits a scope string on ":".
func splitScopeStr(scope string) []string {
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

// --- AI Model DTOs ---

// CreateAIModelRequest is the DTO for creating an AI model.
type CreateAIModelRequest struct {
	Provider            string   `json:"provider"`
	ModelName           string   `json:"model_name"`
	DisplayName         string   `json:"display_name,omitempty"`
	InputPricePerToken  float64  `json:"input_price_per_token"`
	OutputPricePerToken float64  `json:"output_price_per_token"`
	Capabilities        []string `json:"capabilities,omitempty"`
	Weight              int      `json:"weight,omitempty"`
}

// Validate checks required fields.
func (r CreateAIModelRequest) Validate() error {
	if r.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if r.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	return nil
}

// UpdateAIModelRequest is the DTO for updating an AI model.
type UpdateAIModelRequest struct {
	Provider            string   `json:"provider"`
	ModelName           string   `json:"model_name"`
	DisplayName         string   `json:"display_name,omitempty"`
	InputPricePerToken  float64  `json:"input_price_per_token"`
	OutputPricePerToken float64  `json:"output_price_per_token"`
	Capabilities        []string `json:"capabilities,omitempty"`
	IsActive            *bool    `json:"is_active,omitempty"`
	Weight              int      `json:"weight,omitempty"`
}

// Validate checks required fields.
func (r UpdateAIModelRequest) Validate() error {
	if r.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if r.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	return nil
}

// --- Token Budget DTOs ---

// CreateTokenBudgetRequest is the DTO for creating a token budget.
type CreateTokenBudgetRequest struct {
	ConsumerID  *uuid.UUID `json:"consumer_id,omitempty"`
	Team        string     `json:"team,omitempty"`
	Period      string     `json:"period"`
	MaxTokens   *int64     `json:"max_tokens,omitempty"`
	MaxCostUSD  *float64   `json:"max_cost_usd,omitempty"`
	Enforcement string     `json:"enforcement,omitempty"`
}

// Validate checks required fields and constraints.
func (r CreateTokenBudgetRequest) Validate() error {
	if r.ConsumerID == nil && r.Team == "" {
		return fmt.Errorf("at least one of consumer_id or team is required")
	}
	if err := validatePeriod(r.Period); err != nil {
		return err
	}
	return nil
}

// UpdateTokenBudgetRequest is the DTO for updating a token budget.
type UpdateTokenBudgetRequest struct {
	ConsumerID  *uuid.UUID `json:"consumer_id,omitempty"`
	Team        string     `json:"team,omitempty"`
	Period      string     `json:"period"`
	MaxTokens   *int64     `json:"max_tokens,omitempty"`
	MaxCostUSD  *float64   `json:"max_cost_usd,omitempty"`
	Enforcement string     `json:"enforcement,omitempty"`
}

// Validate checks required fields.
func (r UpdateTokenBudgetRequest) Validate() error {
	if r.ConsumerID == nil && r.Team == "" {
		return fmt.Errorf("at least one of consumer_id or team is required")
	}
	if err := validatePeriod(r.Period); err != nil {
		return err
	}
	return nil
}

// --- Response DTOs ---

// RouteResponse is the API response DTO for a route.
type RouteResponse struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Version   int             `json:"version"`
	Config    json.RawMessage `json:"config"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Checksum  string          `json:"checksum"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// RouteResponseFromDomain maps a domain RouteConfig to an API response.
func RouteResponseFromDomain(r domain.RouteConfig) RouteResponse {
	return RouteResponse{
		ID:        r.ID,
		Name:      r.Name,
		Version:   r.Version,
		Config:    r.Config,
		Metadata:  r.Metadata,
		Checksum:  r.Checksum,
		IsActive:  r.IsActive,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// BackendResponse is the API response DTO for a backend.
type BackendResponse struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// BackendResponseFromDomain maps a domain BackendConfig to an API response.
func BackendResponseFromDomain(b domain.BackendConfig) BackendResponse {
	return BackendResponse{
		ID:        b.ID,
		Name:      b.Name,
		Config:    b.Config,
		Metadata:  b.Metadata,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

// ConsumerResponse is the API response DTO for a consumer.
type ConsumerResponse struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ConsumerResponseFromDomain maps a domain Consumer to an API response.
func ConsumerResponseFromDomain(c domain.Consumer) ConsumerResponse {
	return ConsumerResponse{
		ID:        c.ID,
		Name:      c.Name,
		Metadata:  c.Metadata,
		IsActive:  c.IsActive,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// APIKeyResponse is the API response DTO for an API key.
// KeyHash is intentionally excluded from JSON responses.
type APIKeyResponse struct {
	ID                uuid.UUID       `json:"id"`
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

// APIKeyResponseFromDomain maps a domain APIKey to an API response.
func APIKeyResponseFromDomain(k domain.APIKey) APIKeyResponse {
	return APIKeyResponse{
		ID:                k.ID,
		ConsumerID:        k.ConsumerID,
		KeyPrefix:         k.KeyPrefix,
		IsActive:          k.IsActive,
		DeprecatedAt:      k.DeprecatedAt,
		ExpiresAt:         k.ExpiresAt,
		LastUsedAt:        k.LastUsedAt,
		RateLimitOverride: k.RateLimitOverride,
		Permissions:       k.Permissions,
		CreatedAt:         k.CreatedAt,
	}
}

// --- Validation helpers ---

func validateName(name string, minLen, maxLen int) error {
	if len(name) < minLen {
		return fmt.Errorf("name must be between %d and %d characters", minLen, maxLen)
	}
	if len(name) > maxLen {
		return fmt.Errorf("name must be between %d and %d characters", minLen, maxLen)
	}
	return nil
}

func validateJSONRequired(data json.RawMessage, field string) error {
	if len(data) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	if !json.Valid(data) {
		return fmt.Errorf("%s must be valid JSON", field)
	}
	return nil
}

func validatePeriod(period string) error {
	switch period {
	case "hour", "day", "month":
		return nil
	default:
		return fmt.Errorf("period must be one of: hour, day, month")
	}
}
