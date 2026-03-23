package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

func TestValidateRouteConfig(t *testing.T) {
	t.Parallel()

	validConfig := json.RawMessage(`{"path":"/api"}`)

	tests := []struct {
		name    string
		rc      *domain.RouteConfig
		wantErr bool
	}{
		{
			name:    "valid route config",
			rc:      &domain.RouteConfig{Name: "my-route", Config: validConfig},
			wantErr: false,
		},
		{
			name:    "nil route config",
			rc:      nil,
			wantErr: true,
		},
		{
			name:    "empty name",
			rc:      &domain.RouteConfig{Name: "", Config: validConfig},
			wantErr: true,
		},
		{
			name:    "name too long",
			rc:      &domain.RouteConfig{Name: strings.Repeat("a", 256), Config: validConfig},
			wantErr: true,
		},
		{
			name:    "name at max length",
			rc:      &domain.RouteConfig{Name: strings.Repeat("a", 255), Config: validConfig},
			wantErr: false,
		},
		{
			name:    "nil config JSON",
			rc:      &domain.RouteConfig{Name: "my-route", Config: nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRouteConfig(tt.rc)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, domain.ErrInvalidConfig),
					"expected error to wrap ErrInvalidConfig, got: %v", err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBackendConfig(t *testing.T) {
	t.Parallel()

	validConfig := json.RawMessage(`{"url":"http://localhost:8080"}`)

	tests := []struct {
		name    string
		bc      *domain.BackendConfig
		wantErr bool
	}{
		{
			name:    "valid backend config",
			bc:      &domain.BackendConfig{Name: "my-backend", Config: validConfig},
			wantErr: false,
		},
		{
			name:    "nil backend config",
			bc:      nil,
			wantErr: true,
		},
		{
			name:    "empty name",
			bc:      &domain.BackendConfig{Name: "", Config: validConfig},
			wantErr: true,
		},
		{
			name:    "name too long",
			bc:      &domain.BackendConfig{Name: strings.Repeat("b", 256), Config: validConfig},
			wantErr: true,
		},
		{
			name:    "nil config JSON",
			bc:      &domain.BackendConfig{Name: "my-backend", Config: nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateBackendConfig(tt.bc)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, domain.ErrInvalidConfig),
					"expected error to wrap ErrInvalidConfig, got: %v", err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
