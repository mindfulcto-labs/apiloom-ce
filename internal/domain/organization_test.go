package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationStruct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	orgID := uuid.New()

	org := Organization{
		ID:           orgID,
		Name:         "Test Org",
		Slug:         "test-org",
		Metadata:     json.RawMessage(`{"plan":"enterprise"}`),
		Settings:     json.RawMessage(`{"max_routes":100}`),
		BillingEmail: "billing@test.com",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, orgID, org.ID)
	assert.Equal(t, "Test Org", org.Name)
	assert.Equal(t, "test-org", org.Slug)
	assert.Equal(t, "billing@test.com", org.BillingEmail)
	assert.NotNil(t, org.Metadata)
	assert.NotNil(t, org.Settings)
	assert.Equal(t, now, org.CreatedAt)
	assert.Equal(t, now, org.UpdatedAt)

	// Verify JSON round-trip
	data, err := json.Marshal(org)
	require.NoError(t, err)

	var decoded Organization
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, org.ID, decoded.ID)
	assert.Equal(t, org.Name, decoded.Name)
	assert.Equal(t, org.Slug, decoded.Slug)
	assert.Equal(t, org.BillingEmail, decoded.BillingEmail)
}

func TestTeamStruct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	teamID := uuid.New()
	orgID := uuid.New()

	team := Team{
		ID:             teamID,
		OrganizationID: orgID,
		Name:           "Backend Team",
		Metadata:       json.RawMessage(`{"focus":"api"}`),
		CreatedAt:      now,
	}

	assert.Equal(t, teamID, team.ID)
	assert.Equal(t, orgID, team.OrganizationID)
	assert.Equal(t, "Backend Team", team.Name)
	assert.NotNil(t, team.Metadata)
	assert.Equal(t, now, team.CreatedAt)
}

func TestMemberStruct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	memberID := uuid.New()
	orgID := uuid.New()
	teamID := uuid.New()

	member := Member{
		ID:             memberID,
		OrganizationID: orgID,
		TeamID:         &teamID,
		UserID:         "user-abc-123",
		Role:           RoleOrgAdmin,
		CreatedAt:      now,
	}

	assert.Equal(t, memberID, member.ID)
	assert.Equal(t, orgID, member.OrganizationID)
	require.NotNil(t, member.TeamID)
	assert.Equal(t, teamID, *member.TeamID)
	assert.Equal(t, "user-abc-123", member.UserID)
	assert.Equal(t, RoleOrgAdmin, member.Role)
	assert.Equal(t, now, member.CreatedAt)

	// Test nil TeamID
	memberNoTeam := Member{
		ID:             uuid.New(),
		OrganizationID: orgID,
		UserID:         "user-xyz-456",
		Role:           RoleViewer,
		CreatedAt:      now,
	}
	assert.Nil(t, memberNoTeam.TeamID)
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "platform_admin", RolePlatformAdmin)
	assert.Equal(t, "org_admin", RoleOrgAdmin)
	assert.Equal(t, "developer", RoleDeveloper)
	assert.Equal(t, "viewer", RoleViewer)
}

func TestValidateRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{name: "platform_admin is valid", role: RolePlatformAdmin, wantErr: false},
		{name: "org_admin is valid", role: RoleOrgAdmin, wantErr: false},
		{name: "developer is valid", role: RoleDeveloper, wantErr: false},
		{name: "viewer is valid", role: RoleViewer, wantErr: false},
		{name: "empty role is invalid", role: "", wantErr: true},
		{name: "unknown role is invalid", role: "superadmin", wantErr: true},
		{name: "case sensitive", role: "Platform_Admin", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRole(tt.role)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
