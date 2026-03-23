package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

func TestSaveRouteValidation(t *testing.T) {
	t.Parallel()

	// ConfigStore with nil pool -- validation runs before any DB call,
	// so invalid configs should be rejected without touching Postgres.
	store := NewConfigStore(nil)

	tests := []struct {
		name    string
		rc      *domain.RouteConfig
		wantErr error
	}{
		{
			name:    "nil config rejected",
			rc:      nil,
			wantErr: domain.ErrInvalidConfig,
		},
		{
			name:    "empty name rejected",
			rc:      &domain.RouteConfig{Name: "", Config: json.RawMessage(`{}`)},
			wantErr: domain.ErrInvalidConfig,
		},
		{
			name:    "nil JSON rejected",
			rc:      &domain.RouteConfig{Name: "valid-name", Config: nil},
			wantErr: domain.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := store.SaveRoute(t.Context(), tt.rc)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr),
				"expected %v, got: %v", tt.wantErr, err)
		})
	}
}

func TestSaveBackendValidation(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(nil)

	tests := []struct {
		name    string
		bc      *domain.BackendConfig
		wantErr error
	}{
		{
			name:    "nil config rejected",
			bc:      nil,
			wantErr: domain.ErrInvalidConfig,
		},
		{
			name:    "empty name rejected",
			bc:      &domain.BackendConfig{Name: "", Config: json.RawMessage(`{}`)},
			wantErr: domain.ErrInvalidConfig,
		},
		{
			name:    "nil JSON rejected",
			bc:      &domain.BackendConfig{Name: "valid-name", Config: nil},
			wantErr: domain.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := store.SaveBackend(t.Context(), tt.bc)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr),
				"expected %v, got: %v", tt.wantErr, err)
		})
	}
}
