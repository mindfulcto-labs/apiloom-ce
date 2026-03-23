package budget

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentReservesDontOverCommit(t *testing.T) {
	t.Parallel()
	// Budget allows exactly 100 tokens total.
	store := newTestStore(100, 100.0, HardEnforcement, nil)
	ctx := context.Background()
	key := testKey()

	// 20 goroutines each try to reserve 10 tokens.
	// Total requested = 200, but budget is 100. Exactly 10 should succeed.
	var (
		mu       sync.Mutex
		successes int
		failures  int
	)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.Reserve(ctx, key, 10, 0.1)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
			} else {
				successes++
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 10, successes, "exactly 10 reservations should succeed for budget of 100 tokens at 10 each")
	assert.Equal(t, 10, failures, "exactly 10 reservations should fail")
}

func TestReserveThenRefundThenReserveAgain(t *testing.T) {
	t.Parallel()
	store := newTestStore(100, 10.0, HardEnforcement, nil)
	ctx := context.Background()
	key := testKey()

	// Reserve full budget
	err := store.Reserve(ctx, key, 100, 10.0)
	require.NoError(t, err)

	// Budget should be full
	err = store.Reserve(ctx, key, 1, 0.01)
	require.ErrorIs(t, err, ErrBudgetExceeded)

	// Refund half (estimated=100, actual=50)
	err = store.Refund(ctx, key, 100, 50, 5.0)
	require.NoError(t, err)

	// Should now have room for 50 more
	err = store.Reserve(ctx, key, 50, 5.0)
	require.NoError(t, err)
}

func TestBudgetKeyConsumerAndTeam(t *testing.T) {
	t.Parallel()
	store := newTestStore(100, 10.0, HardEnforcement, nil)
	ctx := context.Background()

	cid := uuid.New()
	consumerKey := BudgetKey{ConsumerID: &cid}
	teamKey := BudgetKey{Team: "engineering"}

	// Consumer and team have separate budgets
	err := store.Reserve(ctx, consumerKey, 80, 8.0)
	require.NoError(t, err)

	err = store.Reserve(ctx, teamKey, 80, 8.0)
	require.NoError(t, err)
}
