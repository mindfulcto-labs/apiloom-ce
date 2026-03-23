// Package domain defines core domain types for FlexGateway.
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RequestTrace is a single proxied request observation, captured async off the
// hot path by the TraceCollector block and written to the request_traces table.
type RequestTrace struct {
	OrgID      uuid.UUID  `json:"org_id"`
	ConsumerID *uuid.UUID `json:"consumer_id,omitempty"` // nil if unauthenticated
	RouteID    uuid.UUID  `json:"route_id"`
	BackendID  *uuid.UUID `json:"backend_id,omitempty"`
	Method     string     `json:"method"`
	Path       string     `json:"path"`
	Status     int        `json:"status"`
	LatencyMs  float64    `json:"latency_ms"`
	TracedAt   time.Time  `json:"traced_at"`
}

// IsError returns true when the HTTP status is 400 or above.
func (t *RequestTrace) IsError() bool {
	return t.Status >= 400
}

// Validate returns an error if required fields are missing.
func (t *RequestTrace) Validate() error {
	if t.OrgID == (uuid.UUID{}) {
		return fmt.Errorf("RequestTrace: OrgID is required")
	}
	if t.RouteID == (uuid.UUID{}) {
		return fmt.Errorf("RequestTrace: RouteID is required")
	}
	return nil
}
