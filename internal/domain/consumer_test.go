package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestConsumerStruct(t *testing.T) {
	t.Parallel()

	t.Run("Consumer has correct fields", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		c := Consumer{
			ID:        uuid.New(),
			Name:      "test-consumer",
			Metadata:  json.RawMessage(`{"tier":"premium"}`),
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		assert.NotEqual(t, uuid.Nil, c.ID)
		assert.Equal(t, "test-consumer", c.Name)
		assert.True(t, c.IsActive)
	})
}

func TestAPIKeyStruct(t *testing.T) {
	t.Parallel()

	t.Run("APIKey has correct fields with nullable timestamps", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		deprecatedAt := now.Add(-1 * time.Hour)
		expiresAt := now.Add(24 * time.Hour)

		ak := APIKey{
			ID:                uuid.New(),
			ConsumerID:        uuid.New(),
			KeyHash:           "$2a$12$hash",
			KeyPrefix:         "abcdef01",
			IsActive:          true,
			DeprecatedAt:      &deprecatedAt,
			ExpiresAt:         &expiresAt,
			LastUsedAt:        nil,
			RateLimitOverride: json.RawMessage(`{"rpm":1000}`),
			CreatedAt:         now,
		}
		assert.NotEqual(t, uuid.Nil, ak.ID)
		assert.Equal(t, "abcdef01", ak.KeyPrefix)
		assert.NotNil(t, ak.DeprecatedAt)
		assert.NotNil(t, ak.ExpiresAt)
		assert.Nil(t, ak.LastUsedAt)
	})
}

func TestAPIKeyHasScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permissions json.RawMessage
		scope       string
		want        bool
	}{
		{
			name:        "nil permissions (unrestricted) allows any scope",
			permissions: nil,
			scope:       "api:payments:read",
			want:        true,
		},
		{
			name:        "empty JSON array allows any scope (unrestricted)",
			permissions: json.RawMessage(`[]`),
			scope:       "api:payments:read",
			want:        true,
		},
		{
			name:        "exact scope match returns true",
			permissions: json.RawMessage(`["api:payments:read","api:catalog:write"]`),
			scope:       "api:payments:read",
			want:        true,
		},
		{
			name:        "scope not in list returns false",
			permissions: json.RawMessage(`["api:payments:read"]`),
			scope:       "api:catalog:write",
			want:        false,
		},
		{
			name:        "wildcard in second segment matches",
			permissions: json.RawMessage(`["api:*:read"]`),
			scope:       "api:payments:read",
			want:        true,
		},
		{
			name:        "wildcard in second segment does not match wrong action",
			permissions: json.RawMessage(`["api:*:read"]`),
			scope:       "api:payments:write",
			want:        false,
		},
		{
			name:        "wildcard in third segment matches any action",
			permissions: json.RawMessage(`["api:payments:*"]`),
			scope:       "api:payments:write",
			want:        true,
		},
		{
			name:        "wildcard in third segment matches read",
			permissions: json.RawMessage(`["api:payments:*"]`),
			scope:       "api:payments:read",
			want:        true,
		},
		{
			name:        "full wildcard matches everything",
			permissions: json.RawMessage(`["api:*:*"]`),
			scope:       "api:anything:read",
			want:        true,
		},
		{
			name:        "different segment count does not match",
			permissions: json.RawMessage(`["api:payments"]`),
			scope:       "api:payments:read",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ak := APIKey{Permissions: tt.permissions}
			assert.Equal(t, tt.want, ak.HasScope(tt.scope))
		})
	}
}

func TestAPIKeyIsRestricted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permissions json.RawMessage
		want        bool
	}{
		{
			name:        "nil permissions is unrestricted",
			permissions: nil,
			want:        false,
		},
		{
			name:        "empty array is unrestricted",
			permissions: json.RawMessage(`[]`),
			want:        false,
		},
		{
			name:        "non-empty array is restricted",
			permissions: json.RawMessage(`["api:payments:read"]`),
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ak := APIKey{Permissions: tt.permissions}
			assert.Equal(t, tt.want, ak.IsRestricted())
		})
	}
}
