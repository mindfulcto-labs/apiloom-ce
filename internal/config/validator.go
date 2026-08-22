package config

import (
	"fmt"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// ValidateRouteConfig checks that a RouteConfig has all required fields.
// Returns an error wrapping domain.ErrInvalidConfig if validation fails.
func ValidateRouteConfig(rc *domain.RouteConfig) error {
	if rc == nil {
		return fmt.Errorf("route config is nil: %w", domain.ErrInvalidConfig)
	}
	if rc.Name == "" {
		return fmt.Errorf("route name is required: %w", domain.ErrInvalidConfig)
	}
	if len(rc.Name) > 255 {
		return fmt.Errorf("route name exceeds 255 characters: %w", domain.ErrInvalidConfig)
	}
	if rc.Config == nil {
		return fmt.Errorf("route config JSON is required: %w", domain.ErrInvalidConfig)
	}
	return nil
}

// ValidateBackendConfig checks that a BackendConfig has all required fields.
// Returns an error wrapping domain.ErrInvalidConfig if validation fails.
func ValidateBackendConfig(bc *domain.BackendConfig) error {
	if bc == nil {
		return fmt.Errorf("backend config is nil: %w", domain.ErrInvalidConfig)
	}
	if bc.Name == "" {
		return fmt.Errorf("backend name is required: %w", domain.ErrInvalidConfig)
	}
	if len(bc.Name) > 255 {
		return fmt.Errorf("backend name exceeds 255 characters: %w", domain.ErrInvalidConfig)
	}
	if bc.Config == nil {
		return fmt.Errorf("backend config JSON is required: %w", domain.ErrInvalidConfig)
	}
	return nil
}
