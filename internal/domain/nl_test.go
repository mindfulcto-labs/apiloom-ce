package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNLQueryValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		q       domain.NLQuery
		wantErr bool
	}{
		{
			name:    "valid question",
			q:       domain.NLQuery{Question: "why is payments slow?"},
			wantErr: false,
		},
		{
			name:    "empty question",
			q:       domain.NLQuery{Question: ""},
			wantErr: true,
		},
		{
			name:    "question too long",
			q:       domain.NLQuery{Question: strings.Repeat("a", 2001)},
			wantErr: true,
		},
		{
			name:    "question exactly at limit",
			q:       domain.NLQuery{Question: strings.Repeat("a", 2000)},
			wantErr: false,
		},
		{
			name:    "whitespace only question",
			q:       domain.NLQuery{Question: "   "},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.q.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSuggestedActionRoundtrip(t *testing.T) {
	t.Parallel()

	action := domain.SuggestedAction{
		ID:         "550e8400-e29b-41d4-a716-446655440000",
		Label:      "Rotate API key for consumer acme-corp",
		ActionType: domain.ActionTypeKeyRotation,
		Payload:    json.RawMessage(`{"consumer_id":"abc123","key_id":"key456"}`),
		UndoPayload: json.RawMessage(`{"consumer_id":"abc123","restore_key":"key456"}`),
		RiskLevel:  domain.RiskLevelHigh,
	}

	data, err := json.Marshal(action)
	require.NoError(t, err)

	var decoded domain.SuggestedAction
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, action.ID, decoded.ID)
	assert.Equal(t, action.Label, decoded.Label)
	assert.Equal(t, action.ActionType, decoded.ActionType)
	assert.Equal(t, action.RiskLevel, decoded.RiskLevel)
	assert.Equal(t, string(action.Payload), string(decoded.Payload))
	assert.Equal(t, string(action.UndoPayload), string(decoded.UndoPayload))
}

func TestNLAnswerSuggestedActions(t *testing.T) {
	t.Parallel()

	answer := domain.NLAnswer{
		Answer:     "Payments service is slow due to high latency on upstream backend",
		QueryType:  domain.QueryType("graph"),
		Confidence: 0.87,
		Sources:    []string{"route:payments-api", "backend:payments-service"},
		TurnID:     "550e8400-e29b-41d4-a716-446655440001",
		SuggestedActions: []domain.SuggestedAction{
			{
				ID:          "action-1",
				Label:       "Increase rate limit for payments route",
				ActionType:  domain.ActionTypeRateLimitUpdate,
				Payload:     json.RawMessage(`{"route_id":"pay-1","new_limit":100}`),
				UndoPayload: json.RawMessage(`{"route_id":"pay-1","new_limit":50}`),
				RiskLevel:   domain.RiskLevelLow,
			},
		},
	}

	data, err := json.Marshal(answer)
	require.NoError(t, err)

	var decoded domain.NLAnswer
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, answer.Answer, decoded.Answer)
	assert.Equal(t, answer.QueryType, decoded.QueryType)
	assert.InDelta(t, answer.Confidence, decoded.Confidence, 0.001)
	assert.Equal(t, answer.TurnID, decoded.TurnID)
	require.Len(t, decoded.SuggestedActions, 1)
	assert.Equal(t, domain.RiskLevelLow, decoded.SuggestedActions[0].RiskLevel)
}
