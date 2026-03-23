package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConsumerProfile is a behavioral profile for a (consumer, route) pair,
// computed from aggregated request_traces in 1-minute windows.
// HourlyBuckets[h] counts requests in UTC hour h (0-23).
type ConsumerProfile struct {
	OrgID         uuid.UUID `json:"org_id"`
	ConsumerID    uuid.UUID `json:"consumer_id"`
	RouteID       uuid.UUID `json:"route_id"`
	RequestCount  int64     `json:"request_count"`
	ErrorCount    int64     `json:"error_count"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
	LastSeen      time.Time `json:"last_seen"`
	HourlyBuckets [24]int   `json:"hourly_buckets"`
	WindowStart   time.Time `json:"window_start"`
}

// IncrementHour increments the request count in the given UTC hour bucket.
// Panics if hour is outside [0, 23].
func (p *ConsumerProfile) IncrementHour(hour int) {
	if hour < 0 || hour > 23 {
		panic(fmt.Sprintf("ConsumerProfile.IncrementHour: hour %d out of range [0, 23]", hour))
	}
	p.HourlyBuckets[hour]++
}

// Merge adds another ConsumerProfile's counts into this one, recalculating
// AvgLatencyMs as a weighted average. LastSeen is updated to the later of the two.
func (p *ConsumerProfile) Merge(other *ConsumerProfile) {
	totalRequests := p.RequestCount + other.RequestCount
	if totalRequests > 0 {
		p.AvgLatencyMs = (p.AvgLatencyMs*float64(p.RequestCount) +
			other.AvgLatencyMs*float64(other.RequestCount)) / float64(totalRequests)
	}
	p.RequestCount += other.RequestCount
	p.ErrorCount += other.ErrorCount
	for h := 0; h < 24; h++ {
		p.HourlyBuckets[h] += other.HourlyBuckets[h]
	}
	if other.LastSeen.After(p.LastSeen) {
		p.LastSeen = other.LastSeen
	}
}

// Validate returns an error if required identity fields are zero.
func (p *ConsumerProfile) Validate() error {
	if p.OrgID == (uuid.UUID{}) {
		return fmt.Errorf("ConsumerProfile: OrgID is required")
	}
	if p.ConsumerID == (uuid.UUID{}) {
		return fmt.Errorf("ConsumerProfile: ConsumerID is required")
	}
	if p.RouteID == (uuid.UUID{}) {
		return fmt.Errorf("ConsumerProfile: RouteID is required")
	}
	return nil
}
