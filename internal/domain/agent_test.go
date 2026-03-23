package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent_Validate(t *testing.T) {
	validAgent := func() *Agent {
		return &Agent{
			ID:           uuid.New(),
			OrgID:        uuid.New(),
			Name:         "test-agent",
			OwnerID:      uuid.New(),
			Capabilities: []string{"tool_call"},
			TrustLevel:   TrustBasic,
			Status:       AgentActive,
			AutonomyLevel: AutonomyLow,
			MaxCallDepth:  10,
		}
	}

	tests := []struct {
		name    string
		modify  func(a *Agent)
		wantErr string
	}{
		{
			name:   "valid agent passes validation",
			modify: func(a *Agent) {},
		},
		{
			name:   "valid agent with empty capabilities slice",
			modify: func(a *Agent) { a.Capabilities = []string{} },
		},
		{
			name:    "empty name rejected",
			modify:  func(a *Agent) { a.Name = "" },
			wantErr: "agent name is required",
		},
		{
			name:    "name over 255 chars rejected",
			modify:  func(a *Agent) { a.Name = string(make([]byte, 256)) },
			wantErr: "agent name must be 255 characters or fewer",
		},
		{
			name:    "invalid trust level rejected",
			modify:  func(a *Agent) { a.TrustLevel = "superadmin" },
			wantErr: "invalid trust level",
		},
		{
			name:    "empty string trust level rejected",
			modify:  func(a *Agent) { a.TrustLevel = "" },
			wantErr: "invalid trust level",
		},
		{
			name:    "nil capabilities rejected",
			modify:  func(a *Agent) { a.Capabilities = nil },
			wantErr: "capabilities must not be nil",
		},
		// AGT-01: Autonomy level tests
		{
			name:   "autonomy level low is valid",
			modify: func(a *Agent) { a.AutonomyLevel = AutonomyLow },
		},
		{
			name:   "autonomy level medium is valid",
			modify: func(a *Agent) { a.AutonomyLevel = AutonomyMedium },
		},
		{
			name:   "autonomy level high is valid",
			modify: func(a *Agent) { a.AutonomyLevel = AutonomyHigh },
		},
		{
			name:    "invalid autonomy level rejected",
			modify:  func(a *Agent) { a.AutonomyLevel = "extreme" },
			wantErr: "invalid autonomy level",
		},
		{
			name:   "empty autonomy level defaults to low",
			modify: func(a *Agent) { a.AutonomyLevel = "" },
			// empty defaults to "low" — no error
		},
		// AGT-01: MaxCallDepth tests
		{
			name:   "max_call_depth zero defaults to 10",
			modify: func(a *Agent) { a.MaxCallDepth = 0 },
			// no error — defaults applied
		},
		{
			name:    "max_call_depth over 100 rejected",
			modify:  func(a *Agent) { a.MaxCallDepth = 101 },
			wantErr: "max_call_depth must not exceed 100",
		},
		{
			name:   "max_call_depth of 100 is valid",
			modify: func(a *Agent) { a.MaxCallDepth = 100 },
		},
		{
			name:   "max_call_depth of 1 is valid",
			modify: func(a *Agent) { a.MaxCallDepth = 1 },
		},
		// AGT-01: AllowedTools tests
		{
			name:   "nil allowed tools is valid (no restriction)",
			modify: func(a *Agent) { a.AllowedTools = nil },
		},
		{
			name:   "empty allowed tools slice is valid",
			modify: func(a *Agent) { a.AllowedTools = []string{} },
		},
		{
			name:   "non-empty allowed tools is valid",
			modify: func(a *Agent) { a.AllowedTools = []string{"search", "fetch"} },
		},
		// Existing agent without new fields still validates (backward compat).
		{
			name: "existing agent without new fields defaults gracefully",
			modify: func(a *Agent) {
				a.AutonomyLevel = "" // will default to low
				a.MaxCallDepth = 0  // will default to 10
				a.AllowedTools = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := validAgent()
			tt.modify(a)
			err := a.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				// After validation, AutonomyLevel and MaxCallDepth should be set.
				assert.NotEmpty(t, a.AutonomyLevel, "AutonomyLevel should be set after Validate()")
				assert.Greater(t, a.MaxCallDepth, 0, "MaxCallDepth should be positive after Validate()")
			}
		})
	}
}

func TestTrustLevel_Constants(t *testing.T) {
	assert.Equal(t, TrustLevel("untrusted"), TrustUntrusted)
	assert.Equal(t, TrustLevel("basic"), TrustBasic)
	assert.Equal(t, TrustLevel("elevated"), TrustElevated)
	assert.Equal(t, TrustLevel("admin"), TrustAdmin)
}

func TestAgentStatus_Constants(t *testing.T) {
	assert.Equal(t, AgentStatus("active"), AgentActive)
	assert.Equal(t, AgentStatus("suspended"), AgentSuspended)
	assert.Equal(t, AgentStatus("revoked"), AgentRevoked)
}

func TestAutonomyLevel_Constants(t *testing.T) {
	assert.Equal(t, AutonomyLevel("low"), AutonomyLow)
	assert.Equal(t, AutonomyLevel("medium"), AutonomyMedium)
	assert.Equal(t, AutonomyLevel("high"), AutonomyHigh)
}
