// Package domain contains domain types and sentinel errors shared across FlexGateway.
package domain

import "errors"

// Sentinel errors for business logic. Use errors.Is() to check.
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyExists  = errors.New("already exists")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrRateLimited    = errors.New("rate limited")
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrInvalidConfig  = errors.New("invalid configuration")
	ErrProviderError  = errors.New("upstream provider error")
	ErrCircuitOpen    = errors.New("circuit breaker open")
	ErrInvalidMetric  = errors.New("unknown metric name")
)
