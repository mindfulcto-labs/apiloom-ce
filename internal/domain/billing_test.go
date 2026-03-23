package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageEvent(t *testing.T) {
	t.Parallel()

	t.Run("valid event passes validation", func(t *testing.T) {
		t.Parallel()
		consumerID := uuid.New()
		e := UsageEvent{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			ConsumerID:     &consumerID,
			APIID:          "payments-api",
			Method:         "POST",
			Path:           "/v1/charges",
			StatusCode:     200,
			LatencyMs:      42,
			TokensUsed:     0,
			BytesIn:        128,
			BytesOut:       512,
			CreatedAt:      time.Now(),
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("empty method fails validation", func(t *testing.T) {
		t.Parallel()
		e := UsageEvent{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			Method:         "",
		}
		err := e.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method")
	})

	t.Run("nil consumer ID is valid for unauthenticated calls", func(t *testing.T) {
		t.Parallel()
		e := UsageEvent{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			Method:         "GET",
			Path:           "/health",
		}
		assert.NoError(t, e.Validate())
	})

	t.Run("sent_at is nullable", func(t *testing.T) {
		t.Parallel()
		e := UsageEvent{
			ID:     uuid.New(),
			Method: "GET",
		}
		assert.Nil(t, e.SentAt)

		now := time.Now()
		e.SentAt = &now
		assert.NotNil(t, e.SentAt)
	})
}

func TestPlanTierConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier PlanTier
		want string
	}{
		{PlanCommunity, "community"},
		{PlanPro, "pro"},
		{PlanBusiness, "business"},
		{PlanEnterprise, "enterprise"},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(tt.tier))
		})
	}
}

func TestSubscriptionPlan(t *testing.T) {
	t.Parallel()

	t.Run("plan struct has expected fields", func(t *testing.T) {
		t.Parallel()
		p := SubscriptionPlan{
			ID:           uuid.New(),
			Name:         "Pro",
			Slug:         "pro",
			Tier:         PlanPro,
			PriceMonthly: 9900,
			RequestLimit: 500000,
			RateLimitRPS: 100,
			Features:     json.RawMessage(`{"support":"email"}`),
			IsActive:     true,
			CreatedAt:    time.Now(),
		}
		assert.Equal(t, "Pro", p.Name)
		assert.Equal(t, 9900, p.PriceMonthly)
		assert.Equal(t, int64(500000), p.RequestLimit)
	})
}

func TestSubscription(t *testing.T) {
	t.Parallel()

	t.Run("subscription struct has expected fields", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		s := Subscription{
			ID:                   uuid.New(),
			OrganizationID:       uuid.New(),
			PlanID:               uuid.New(),
			StripeSubscriptionID: "sub_12345",
			StripeCustomerID:     "cus_12345",
			Status:               "active",
			CurrentPeriodStart:   now,
			CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		assert.Equal(t, "active", s.Status)
		assert.True(t, s.CurrentPeriodEnd.After(s.CurrentPeriodStart))
	})
}

func TestBudgetAlert(t *testing.T) {
	t.Parallel()

	t.Run("budget alert struct has expected fields", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		a := BudgetAlert{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			Threshold:      80,
			PeriodStart:    now,
		}
		assert.Equal(t, 80, a.Threshold)
		assert.Nil(t, a.NotifiedAt)
	})
}
