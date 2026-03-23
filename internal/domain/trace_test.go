package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestTrace_IsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "200 OK is not error", status: 200, wantErr: false},
		{name: "201 Created is not error", status: 201, wantErr: false},
		{name: "301 redirect is not error", status: 301, wantErr: false},
		{name: "399 is not error", status: 399, wantErr: false},
		{name: "400 Bad Request is error", status: 400, wantErr: true},
		{name: "401 Unauthorized is error", status: 401, wantErr: true},
		{name: "403 Forbidden is error", status: 403, wantErr: true},
		{name: "404 Not Found is error", status: 404, wantErr: true},
		{name: "500 Internal Server Error is error", status: 500, wantErr: true},
		{name: "503 Service Unavailable is error", status: 503, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trace := &RequestTrace{
				OrgID:   uuid.New(),
				RouteID: uuid.New(),
				Method:  "GET",
				Path:    "/test",
				Status:  tt.status,
				TracedAt: time.Now(),
			}
			assert.Equal(t, tt.wantErr, trace.IsError())
		})
	}
}

func TestRequestTrace_Validate(t *testing.T) {
	t.Parallel()

	validOrgID := uuid.New()
	validRouteID := uuid.New()
	consumerID := uuid.New()
	backendID := uuid.New()

	tests := []struct {
		name    string
		trace   *RequestTrace
		wantErr bool
	}{
		{
			name: "fully populated struct is valid",
			trace: &RequestTrace{
				OrgID:      validOrgID,
				ConsumerID: &consumerID,
				RouteID:    validRouteID,
				BackendID:  &backendID,
				Method:     "POST",
				Path:       "/api/v1/users",
				Status:     201,
				LatencyMs:  42.5,
				TracedAt:   time.Now(),
			},
			wantErr: false,
		},
		{
			name: "nil consumer_id is valid (unauthenticated)",
			trace: &RequestTrace{
				OrgID:     validOrgID,
				RouteID:   validRouteID,
				Method:    "GET",
				Path:      "/public",
				Status:    200,
				LatencyMs: 10.0,
				TracedAt:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "zero OrgID returns error",
			trace: &RequestTrace{
				OrgID:     uuid.UUID{},
				RouteID:   validRouteID,
				Method:    "GET",
				Path:      "/test",
				Status:    200,
				LatencyMs: 5.0,
				TracedAt:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero RouteID returns error",
			trace: &RequestTrace{
				OrgID:     validOrgID,
				RouteID:   uuid.UUID{},
				Method:    "GET",
				Path:      "/test",
				Status:    200,
				LatencyMs: 5.0,
				TracedAt:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "both OrgID and RouteID zero returns error",
			trace: &RequestTrace{
				OrgID:     uuid.UUID{},
				RouteID:   uuid.UUID{},
				Method:    "GET",
				Path:      "/test",
				Status:    200,
				LatencyMs: 5.0,
				TracedAt:  time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.trace.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
