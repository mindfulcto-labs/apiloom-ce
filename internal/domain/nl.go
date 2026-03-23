// Package domain contains domain types and sentinel errors shared across FlexGateway.
package domain

import (
	"encoding/json"
	"errors"
	"strings"
)

// ActionType defines the category of a suggested NL action.
type ActionType string

const (
	// ActionTypeConfigChange represents a configuration change action.
	ActionTypeConfigChange ActionType = "config_change"
	// ActionTypeKeyRotation represents a key rotation action.
	ActionTypeKeyRotation ActionType = "key_rotation"
	// ActionTypeRateLimitUpdate represents a rate limit update action.
	ActionTypeRateLimitUpdate ActionType = "rate_limit_update"
	// ActionTypeConsumerSuspend represents a consumer suspension action.
	ActionTypeConsumerSuspend ActionType = "consumer_suspend"
	// ActionTypeRouteDeploy represents a route deployment action.
	ActionTypeRouteDeploy ActionType = "route_deploy"
)

// RiskLevel indicates the operational risk of executing a suggested action.
type RiskLevel string

const (
	// RiskLevelLow indicates a low-risk action that can be confirmed with a single click.
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium indicates a medium-risk action that should be reviewed before applying.
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh indicates a high-risk action requiring explicit typed confirmation.
	RiskLevelHigh RiskLevel = "high"
)

// NLQueryType is an alias for the intelligence query type classification.
// Uses the existing QueryType from intelligence.go (graph | metric | hybrid).
type NLQueryType = QueryType

// SuggestedAction is an actionable mutation derived from an NL query response.
// The UndoPayload allows reversing the action if needed.
type SuggestedAction struct {
	// ID is a UUID identifying this action instance.
	ID string `json:"id"`
	// Label is a human-readable description of the action.
	Label string `json:"label"`
	// ActionType categorizes the mutation.
	ActionType ActionType `json:"action_type"`
	// Payload is the opaque JSON body sent to the execute endpoint.
	Payload json.RawMessage `json:"payload"`
	// UndoPayload is the opaque JSON body used to reverse the action.
	UndoPayload json.RawMessage `json:"undo_payload"`
	// RiskLevel indicates how dangerous this action is.
	RiskLevel RiskLevel `json:"risk_level"`
}

// NLAnswer is the response from the NL query engine.
type NLAnswer struct {
	// Answer is the human-readable response text.
	Answer string `json:"answer"`
	// QueryType describes how the answer was derived.
	QueryType NLQueryType `json:"query_type"`
	// Confidence is the model's confidence score in the range [0.0, 1.0].
	Confidence float64 `json:"confidence"`
	// Sources are identifiers of graph nodes or metrics that contributed to the answer.
	Sources []string `json:"sources,omitempty"`
	// SuggestedActions are mutations the system recommends based on the answer.
	SuggestedActions []SuggestedAction `json:"suggested_actions"`
	// TurnID is a UUID used to thread multi-turn conversations.
	TurnID string `json:"turn_id"`
}

// NLQuery is the request body for the NL query endpoint.
type NLQuery struct {
	// Question is the natural language question (max 2000 chars).
	Question string `json:"question"`
	// Context provides optional key/value context for query disambiguation.
	Context map[string]string `json:"context,omitempty"`
	// TurnID is the turn_id from the previous assistant response for multi-turn threading.
	TurnID string `json:"turn_id,omitempty"`
}

// Validate checks that the NLQuery meets input constraints.
func (q NLQuery) Validate() error {
	trimmed := strings.TrimSpace(q.Question)
	if trimmed == "" {
		return errors.New("question is required and must not be blank")
	}
	if len(q.Question) > 2000 {
		return errors.New("question must not exceed 2000 characters")
	}
	return nil
}

// QueryHistoryEntry records a Q+A pair for history persistence.
type QueryHistoryEntry struct {
	// ID is a UUID for the history entry.
	ID string `json:"id"`
	// UserID identifies the user who asked the question.
	UserID string `json:"user_id"`
	// OrgID identifies the organization context.
	OrgID string `json:"org_id"`
	// Question is the original query text.
	Question string `json:"question"`
	// Answer is the full NLAnswer returned.
	Answer NLAnswer `json:"answer"`
	// CreatedAt is the RFC 3339 timestamp of the query.
	CreatedAt string `json:"created_at"`
}

// SavedQuery is a user-named query that persists across sessions.
type SavedQuery struct {
	// ID is a UUID for the saved query.
	ID string `json:"id"`
	// UserID identifies the user who saved the query.
	UserID string `json:"user_id"`
	// OrgID identifies the organization context.
	OrgID string `json:"org_id"`
	// Name is the user-assigned label for the saved query.
	Name string `json:"name"`
	// Question is the query text to re-execute.
	Question string `json:"question"`
	// CreatedAt is the RFC 3339 timestamp when the query was saved.
	CreatedAt string `json:"created_at"`
}
