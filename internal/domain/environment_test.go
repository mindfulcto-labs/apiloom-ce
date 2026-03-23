package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironment_Validate(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()

	tests := []struct {
		name    string
		env     Environment
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid environment",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "Development",
			},
			wantErr: false,
		},
		{
			name: "valid with explicit slug",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "My Custom Env",
				Slug:           "my-custom-env",
			},
			wantErr: false,
		},
		{
			name: "name too short",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "x",
			},
			wantErr: true,
			errMsg:  "2-50 characters",
		},
		{
			name: "name too long",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "a very long environment name that exceeds the fifty character maximum limit",
			},
			wantErr: true,
			errMsg:  "2-50 characters",
		},
		{
			name: "empty name",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "",
			},
			wantErr: true,
			errMsg:  "2-50 characters",
		},
		{
			name: "whitespace-only name",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "   ",
			},
			wantErr: true,
			errMsg:  "2-50 characters",
		},
		{
			name: "auto-generates slug from name",
			env: Environment{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Name:           "My Custom Env",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := tt.env
			err := env.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, env.Slug, "slug should be populated after validation")
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Development", "development"},
		{"My Custom Env", "my-custom-env"},
		{"  Staging  ", "staging"},
		{"Production!@#$%", "production"},
		{"multi---dash", "multi-dash"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := Slugify(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultEnvironments(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	envs := DefaultEnvironments(orgID)

	require.Len(t, envs, 3)

	assert.Equal(t, "Development", envs[0].Name)
	assert.Equal(t, "development", envs[0].Slug)
	assert.False(t, envs[0].IsProduction)

	assert.Equal(t, "Staging", envs[1].Name)
	assert.Equal(t, "staging", envs[1].Slug)
	assert.False(t, envs[1].IsProduction)

	assert.Equal(t, "Production", envs[2].Name)
	assert.Equal(t, "production", envs[2].Slug)
	assert.True(t, envs[2].IsProduction)

	for _, env := range envs {
		assert.Equal(t, orgID, env.OrganizationID)
		assert.NotEqual(t, uuid.Nil, env.ID)
	}
}
