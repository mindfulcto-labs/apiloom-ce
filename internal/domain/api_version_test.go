package domain

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIVersion_Validate(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()

	tests := []struct {
		name    string
		version APIVersion
		wantErr bool
	}{
		{
			name:    "valid version",
			version: APIVersion{RouteID: routeID, VersionTag: "v1.0.0"},
			wantErr: false,
		},
		{
			name:    "valid with leading space",
			version: APIVersion{RouteID: routeID, VersionTag: " v2.3.10 "},
			wantErr: false,
		},
		{
			name:    "missing route_id",
			version: APIVersion{VersionTag: "v1.0.0"},
			wantErr: true,
		},
		{
			name:    "missing v prefix",
			version: APIVersion{RouteID: routeID, VersionTag: "1.0.0"},
			wantErr: true,
		},
		{
			name:    "invalid semver",
			version: APIVersion{RouteID: routeID, VersionTag: "v1.0"},
			wantErr: true,
		},
		{
			name:    "empty tag",
			version: APIVersion{RouteID: routeID, VersionTag: ""},
			wantErr: true,
		},
		{
			name:    "leading zero in major",
			version: APIVersion{RouteID: routeID, VersionTag: "v01.0.0"},
			wantErr: true,
		},
		{
			name:    "v0.0.0 is valid",
			version: APIVersion{RouteID: routeID, VersionTag: "v0.0.0"},
			wantErr: false,
		},
		{
			name:    "large version numbers",
			version: APIVersion{RouteID: routeID, VersionTag: "v100.200.300"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.version.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCanaryConfig_Validate(t *testing.T) {
	t.Parallel()

	routeID := uuid.New()
	v1 := uuid.New()
	v2 := uuid.New()

	tests := []struct {
		name    string
		config  CanaryConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: 25},
			wantErr: false,
		},
		{
			name:    "weight 0 is valid",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: 0},
			wantErr: false,
		},
		{
			name:    "weight 100 is valid",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: 100},
			wantErr: false,
		},
		{
			name:    "weight negative",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: -1},
			wantErr: true,
		},
		{
			name:    "weight over 100",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: 101},
			wantErr: true,
		},
		{
			name:    "missing route_id",
			config:  CanaryConfig{PrimaryVersionID: v1, CanaryVersionID: v2, CanaryWeight: 50},
			wantErr: true,
		},
		{
			name:    "same primary and canary",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryVersionID: v1, CanaryWeight: 50},
			wantErr: true,
		},
		{
			name:    "missing canary_version_id",
			config:  CanaryConfig{RouteID: routeID, PrimaryVersionID: v1, CanaryWeight: 50},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDiffConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		oldConfig   string
		newConfig   string
		wantAdded   []string
		wantRemoved []string
		wantChanged int
	}{
		{
			name:        "identical configs",
			oldConfig:   `{"timeout": 30, "retries": 3}`,
			newConfig:   `{"timeout": 30, "retries": 3}`,
			wantAdded:   []string{},
			wantRemoved: []string{},
			wantChanged: 0,
		},
		{
			name:        "added field",
			oldConfig:   `{"timeout": 30}`,
			newConfig:   `{"timeout": 30, "retry_count": 3}`,
			wantAdded:   []string{"retry_count"},
			wantRemoved: []string{},
			wantChanged: 0,
		},
		{
			name:        "removed field",
			oldConfig:   `{"timeout": 30, "deprecated": true}`,
			newConfig:   `{"timeout": 30}`,
			wantAdded:   []string{},
			wantRemoved: []string{"deprecated"},
			wantChanged: 0,
		},
		{
			name:        "changed field",
			oldConfig:   `{"timeout": 30}`,
			newConfig:   `{"timeout": 60}`,
			wantAdded:   []string{},
			wantRemoved: []string{},
			wantChanged: 1,
		},
		{
			name:        "nested changes",
			oldConfig:   `{"settings": {"timeout": 30, "old_field": true}}`,
			newConfig:   `{"settings": {"timeout": 60, "new_field": "hello"}}`,
			wantAdded:   []string{"settings.new_field"},
			wantRemoved: []string{"settings.old_field"},
			wantChanged: 1,
		},
		{
			name:        "multiple changes",
			oldConfig:   `{"a": 1, "b": 2, "c": 3}`,
			newConfig:   `{"a": 10, "c": 3, "d": 4}`,
			wantAdded:   []string{"d"},
			wantRemoved: []string{"b"},
			wantChanged: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			diff, err := DiffConfigs(json.RawMessage(tt.oldConfig), json.RawMessage(tt.newConfig))
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantAdded, diff.Added)
			assert.ElementsMatch(t, tt.wantRemoved, diff.Removed)
			assert.Len(t, diff.Changed, tt.wantChanged)
		})
	}
}

func TestDiffConfigs_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := DiffConfigs(json.RawMessage(`not json`), json.RawMessage(`{}`))
	require.Error(t, err)

	_, err = DiffConfigs(json.RawMessage(`{}`), json.RawMessage(`not json`))
	require.Error(t, err)
}

func TestIsBreakingChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		diff     ConfigDiff
		breaking bool
	}{
		{
			name:     "no changes",
			diff:     ConfigDiff{Added: []string{}, Removed: []string{}, Changed: []FieldChange{}},
			breaking: false,
		},
		{
			name:     "only additions",
			diff:     ConfigDiff{Added: []string{"new_field"}, Removed: []string{}, Changed: []FieldChange{}},
			breaking: false,
		},
		{
			name:     "removed field is breaking",
			diff:     ConfigDiff{Added: []string{}, Removed: []string{"old_field"}, Changed: []FieldChange{}},
			breaking: true,
		},
		{
			name: "type change is breaking",
			diff: ConfigDiff{
				Added: []string{}, Removed: []string{},
				Changed: []FieldChange{{Field: "timeout", OldValue: "30", NewValue: `"thirty"`}},
			},
			breaking: true,
		},
		{
			name: "same type change is not breaking",
			diff: ConfigDiff{
				Added: []string{}, Removed: []string{},
				Changed: []FieldChange{{Field: "timeout", OldValue: "30", NewValue: "60"}},
			},
			breaking: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.breaking, IsBreakingChange(tt.diff))
		})
	}
}

func TestGenerateMigrationNotes(t *testing.T) {
	t.Parallel()

	diff := ConfigDiff{
		Added:   []string{"retry_count"},
		Removed: []string{"timeout"},
		Changed: []FieldChange{
			{Field: "max_conns", OldValue: "10", NewValue: "20"},
			{Field: "mode", OldValue: `"sync"`, NewValue: "true"},
		},
	}

	notes := GenerateMigrationNotes(diff)
	require.Len(t, notes, 4)

	assert.Contains(t, notes[0], "timeout")
	assert.Contains(t, notes[0], "BREAKING")
	assert.Contains(t, notes[1], "retry_count")
	assert.Contains(t, notes[1], "added")
	assert.Contains(t, notes[2], "max_conns")
	assert.Contains(t, notes[2], "modified")
	assert.Contains(t, notes[3], "mode")
	assert.Contains(t, notes[3], "BREAKING")
}
