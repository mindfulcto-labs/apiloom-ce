package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumerProfile_IncrementHour(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		hour        int
		wantBucket  int
		shouldPanic bool
	}{
		{name: "hour 0 increments bucket 0", hour: 0, wantBucket: 0, shouldPanic: false},
		{name: "hour 12 increments bucket 12", hour: 12, wantBucket: 12, shouldPanic: false},
		{name: "hour 23 increments bucket 23", hour: 23, wantBucket: 23, shouldPanic: false},
		{name: "hour -1 panics", hour: -1, shouldPanic: true},
		{name: "hour 24 panics", hour: 24, shouldPanic: true},
		{name: "hour 100 panics", hour: 100, shouldPanic: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &ConsumerProfile{
				OrgID:      uuid.New(),
				ConsumerID: uuid.New(),
				RouteID:    uuid.New(),
			}
			if tt.shouldPanic {
				assert.Panics(t, func() { p.IncrementHour(tt.hour) })
			} else {
				p.IncrementHour(tt.hour)
				assert.Equal(t, 1, p.HourlyBuckets[tt.wantBucket])
				// Verify no other buckets were incremented
				for h := 0; h < 24; h++ {
					if h == tt.wantBucket {
						continue
					}
					assert.Equal(t, 0, p.HourlyBuckets[h], "bucket %d should be 0", h)
				}
			}
		})
	}
}

func TestConsumerProfile_IncrementHour_Accumulates(t *testing.T) {
	t.Parallel()

	p := &ConsumerProfile{
		OrgID:      uuid.New(),
		ConsumerID: uuid.New(),
		RouteID:    uuid.New(),
	}

	// Increment hour 5 three times
	p.IncrementHour(5)
	p.IncrementHour(5)
	p.IncrementHour(5)

	assert.Equal(t, 3, p.HourlyBuckets[5])
}

func TestConsumerProfile_Merge(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	consumerID := uuid.New()
	routeID := uuid.New()
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	tests := []struct {
		name              string
		base              *ConsumerProfile
		other             *ConsumerProfile
		wantRequestCount  int64
		wantErrorCount    int64
		wantAvgLatency    float64
		wantLastSeen      time.Time
		wantHourlyBucket5 int
	}{
		{
			name: "merge adds counts and recalculates weighted avg latency",
			base: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 100,
				ErrorCount:   5,
				AvgLatencyMs: 50.0,
				LastSeen:     earlier,
				HourlyBuckets: func() [24]int {
					var b [24]int
					b[5] = 10
					return b
				}(),
			},
			other: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 100,
				ErrorCount:   10,
				AvgLatencyMs: 100.0,
				LastSeen:     now,
				HourlyBuckets: func() [24]int {
					var b [24]int
					b[5] = 5
					return b
				}(),
			},
			wantRequestCount:  200,
			wantErrorCount:    15,
			wantAvgLatency:    75.0, // (50*100 + 100*100) / 200
			wantLastSeen:      now,
			wantHourlyBucket5: 15,
		},
		{
			name: "merge into empty base uses other avg latency",
			base: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 0,
				ErrorCount:   0,
				AvgLatencyMs: 0,
				LastSeen:     earlier,
			},
			other: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 50,
				ErrorCount:   2,
				AvgLatencyMs: 30.0,
				LastSeen:     now,
			},
			wantRequestCount:  50,
			wantErrorCount:    2,
			wantAvgLatency:    30.0,
			wantLastSeen:      now,
			wantHourlyBucket5: 0,
		},
		{
			name: "earlier last_seen keeps base last_seen",
			base: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 10,
				ErrorCount:   0,
				AvgLatencyMs: 20.0,
				LastSeen:     now,
			},
			other: &ConsumerProfile{
				OrgID:        orgID,
				ConsumerID:   consumerID,
				RouteID:      routeID,
				RequestCount: 10,
				ErrorCount:   0,
				AvgLatencyMs: 20.0,
				LastSeen:     earlier,
			},
			wantRequestCount:  20,
			wantErrorCount:    0,
			wantAvgLatency:    20.0,
			wantLastSeen:      now,
			wantHourlyBucket5: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.base.Merge(tt.other)
			assert.Equal(t, tt.wantRequestCount, tt.base.RequestCount)
			assert.Equal(t, tt.wantErrorCount, tt.base.ErrorCount)
			assert.InDelta(t, tt.wantAvgLatency, tt.base.AvgLatencyMs, 0.001)
			assert.Equal(t, tt.wantLastSeen, tt.base.LastSeen)
			assert.Equal(t, tt.wantHourlyBucket5, tt.base.HourlyBuckets[5])
		})
	}
}

func TestConsumerProfile_Validate(t *testing.T) {
	t.Parallel()

	validOrgID := uuid.New()
	validConsumerID := uuid.New()
	validRouteID := uuid.New()

	tests := []struct {
		name    string
		profile *ConsumerProfile
		wantErr bool
	}{
		{
			name: "fully populated is valid",
			profile: &ConsumerProfile{
				OrgID:        validOrgID,
				ConsumerID:   validConsumerID,
				RouteID:      validRouteID,
				RequestCount: 10,
				LastSeen:     time.Now(),
			},
			wantErr: false,
		},
		{
			name: "zero OrgID returns error",
			profile: &ConsumerProfile{
				OrgID:      uuid.UUID{},
				ConsumerID: validConsumerID,
				RouteID:    validRouteID,
			},
			wantErr: true,
		},
		{
			name: "zero ConsumerID returns error",
			profile: &ConsumerProfile{
				OrgID:      validOrgID,
				ConsumerID: uuid.UUID{},
				RouteID:    validRouteID,
			},
			wantErr: true,
		},
		{
			name: "zero RouteID returns error",
			profile: &ConsumerProfile{
				OrgID:      validOrgID,
				ConsumerID: validConsumerID,
				RouteID:    uuid.UUID{},
			},
			wantErr: true,
		},
		{
			name: "all zero UUIDs return error",
			profile: &ConsumerProfile{
				OrgID:      uuid.UUID{},
				ConsumerID: uuid.UUID{},
				RouteID:    uuid.UUID{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.profile.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
