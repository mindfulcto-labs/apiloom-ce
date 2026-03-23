package config

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackoff_ExponentialWithCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{name: "attempt 0", attempt: 0, wantMin: 1 * time.Second, wantMax: 2 * time.Second},
		{name: "attempt 1", attempt: 1, wantMin: 2 * time.Second, wantMax: 3 * time.Second},
		{name: "attempt 2", attempt: 2, wantMin: 4 * time.Second, wantMax: 5 * time.Second},
		{name: "attempt 3", attempt: 3, wantMin: 8 * time.Second, wantMax: 9 * time.Second},
		{name: "attempt 10 capped at 30s", attempt: 10, wantMin: 30 * time.Second, wantMax: 31 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := backoff(tt.attempt)
			assert.GreaterOrEqual(t, d, tt.wantMin, "backoff(%d) too low", tt.attempt)
			assert.LessOrEqual(t, d, tt.wantMax, "backoff(%d) too high", tt.attempt)
		})
	}
}

func TestBackoff_JitterVaries(t *testing.T) {
	t.Parallel()

	// Run backoff 20 times and confirm we get at least 2 distinct values
	// (jitter should produce variation).
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 20; i++ {
		seen[backoff(1)] = struct{}{}
	}
	require.Greater(t, len(seen), 1, "backoff should produce varying durations due to jitter")
}

func TestDebounce_CoalescesMultipleCalls(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	fn := func() { callCount.Add(1) }

	d := newDebouncer(50*time.Millisecond, fn)

	// Trigger multiple times rapidly.
	d.trigger()
	d.trigger()
	d.trigger()

	// Wait for debounce to fire.
	require.Eventually(t, func() bool {
		return callCount.Load() == 1
	}, 500*time.Millisecond, 10*time.Millisecond, "debounced function should fire exactly once")

	// Verify it was exactly 1 call (not more after settling).
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), callCount.Load(), "expected exactly 1 debounced call")
}

func TestDebounce_FiresAgainAfterInterval(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	fn := func() { callCount.Add(1) }

	d := newDebouncer(50*time.Millisecond, fn)

	d.trigger()
	require.Eventually(t, func() bool {
		return callCount.Load() == 1
	}, 500*time.Millisecond, 10*time.Millisecond)

	// Wait past debounce window, trigger again.
	time.Sleep(100 * time.Millisecond)
	d.trigger()
	require.Eventually(t, func() bool {
		return callCount.Load() == 2
	}, 500*time.Millisecond, 10*time.Millisecond, "debouncer should fire again after interval")
}
