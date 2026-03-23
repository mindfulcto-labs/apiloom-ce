package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UsageEvent represents a single metered API call written to the billing outbox.
// Events are durable (Postgres-backed) and survive gateway crashes.
type UsageEvent struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ConsumerID     *uuid.UUID `json:"consumer_id,omitempty"`
	APIID          string     `json:"api_id"`
	Method         string     `json:"method"`
	Path           string     `json:"path"`
	StatusCode     int        `json:"status_code"`
	LatencyMs      int64      `json:"latency_ms"`
	TokensUsed     int64      `json:"tokens_used"`
	BytesIn        int64      `json:"bytes_in"`
	BytesOut       int64      `json:"bytes_out"`
	CreatedAt      time.Time  `json:"created_at"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
}

// Validate returns an error if required fields are missing.
func (e *UsageEvent) Validate() error {
	if e.Method == "" {
		return fmt.Errorf("validating usage event: method is required")
	}
	return nil
}

// PlanTier represents the tier level of a subscription plan.
type PlanTier string

const (
	PlanCommunity  PlanTier = "community"
	PlanPro        PlanTier = "pro"
	PlanBusiness   PlanTier = "business"
	PlanEnterprise PlanTier = "enterprise"
)

// SubscriptionPlan represents an available subscription plan with pricing and limits.
type SubscriptionPlan struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Tier         PlanTier        `json:"tier"`
	PriceMonthly int             `json:"price_monthly"` // cents
	RequestLimit int64           `json:"request_limit"`
	RateLimitRPS int             `json:"rate_limit_rps"`
	Features     json.RawMessage `json:"features,omitempty"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Subscription represents an organization's active subscription to a plan.
type Subscription struct {
	ID                   uuid.UUID  `json:"id"`
	OrganizationID       uuid.UUID  `json:"organization_id"`
	PlanID               uuid.UUID  `json:"plan_id"`
	StripeSubscriptionID string     `json:"stripe_subscription_id,omitempty"`
	StripeCustomerID     string     `json:"stripe_customer_id,omitempty"`
	Status               string     `json:"status"`
	CurrentPeriodStart   time.Time  `json:"current_period_start"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// BudgetAlert records that an organization was notified at a usage threshold.
type BudgetAlert struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Threshold      int        `json:"threshold"`
	NotifiedAt     *time.Time `json:"notified_at,omitempty"`
	PeriodStart    time.Time  `json:"period_start"`
	CreatedAt      time.Time  `json:"created_at"`
}
