package budget

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetKeyString(t *testing.T) {
	t.Parallel()

	cid := uuid.New()
	tests := []struct {
		name string
		key  BudgetKey
		want string
	}{
		{name: "consumer key", key: BudgetKey{ConsumerID: &cid}, want: "consumer:" + cid.String()},
		{name: "team key", key: BudgetKey{Team: "engineering"}, want: "team:engineering"},
		{name: "both prefers consumer", key: BudgetKey{ConsumerID: &cid, Team: "eng"}, want: "consumer:" + cid.String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.key.String())
		})
	}
}

func TestMemoryStoreReserveWithinBudget(t *testing.T) {
	t.Parallel()
	store := newTestStore(1000, 10.0, HardEnforcement, nil)
	ctx := context.Background()

	err := store.Reserve(ctx, testKey(), 100, 1.0)
	require.NoError(t, err)
}

func TestMemoryStoreReserveExceedsTokenLimit(t *testing.T) {
	t.Parallel()
	store := newTestStore(100, 100.0, HardEnforcement, nil)
	ctx := context.Background()

	err := store.Reserve(ctx, testKey(), 200, 1.0)
	require.ErrorIs(t, err, ErrBudgetExceeded)
}

func TestMemoryStoreReserveExceedsCostLimit(t *testing.T) {
	t.Parallel()
	store := newTestStore(10000, 1.0, HardEnforcement, nil)
	ctx := context.Background()

	err := store.Reserve(ctx, testKey(), 100, 2.0)
	require.ErrorIs(t, err, ErrBudgetExceeded)
}

func TestMemoryStoreRefundCreditsBack(t *testing.T) {
	t.Parallel()
	store := newTestStore(1000, 10.0, HardEnforcement, nil)
	ctx := context.Background()
	key := testKey()

	// Reserve 500 tokens
	err := store.Reserve(ctx, key, 500, 5.0)
	require.NoError(t, err)

	// Refund: estimated=500, actual=300. Should credit back 200.
	err = store.Refund(ctx, key, 500, 300, 3.0)
	require.NoError(t, err)

	// Should now have 300 tokens used, 7.0 cost left -> can reserve 600 more
	err = store.Reserve(ctx, key, 600, 2.0)
	require.NoError(t, err)
}

func TestMemoryStoreRefundNeverNegative(t *testing.T) {
	t.Parallel()
	store := newTestStore(1000, 10.0, HardEnforcement, nil)
	ctx := context.Background()
	key := testKey()

	// Reserve 50 tokens
	err := store.Reserve(ctx, key, 50, 0.5)
	require.NoError(t, err)

	// Refund more than reserved (estimated=200, actual=10)
	err = store.Refund(ctx, key, 200, 10, 0.1)
	require.NoError(t, err)

	// Should be clamped to 0 tokens used, can use full budget
	err = store.Reserve(ctx, key, 1000, 9.5)
	require.NoError(t, err)
}

func TestMemoryStoreCheckCeilingExceeded(t *testing.T) {
	t.Parallel()
	ceiling := 5.0
	store := newTestStore(10000, 100.0, HardEnforcement, &ceiling)
	ctx := context.Background()

	err := store.CheckCeiling(ctx, testKey(), 10.0)
	require.ErrorIs(t, err, ErrCeilingExceeded)
}

func TestMemoryStoreCheckCeilingNoCeiling(t *testing.T) {
	t.Parallel()
	store := newTestStore(10000, 100.0, HardEnforcement, nil)
	ctx := context.Background()

	err := store.CheckCeiling(ctx, testKey(), 999.0)
	require.NoError(t, err)
}

func TestMemoryStoreCheckCeilingWithinLimit(t *testing.T) {
	t.Parallel()
	ceiling := 50.0
	store := newTestStore(10000, 100.0, HardEnforcement, &ceiling)
	ctx := context.Background()

	err := store.CheckCeiling(ctx, testKey(), 10.0)
	require.NoError(t, err)
}

func TestMemoryStoreSoftEnforcementAllows(t *testing.T) {
	t.Parallel()
	store := newTestStore(100, 1.0, SoftEnforcement, nil)
	ctx := context.Background()

	result, err := store.ReserveWithResult(ctx, testKey(), 200, 2.0)
	require.NoError(t, err)
	assert.True(t, result.Warning, "soft enforcement should set warning flag")
	assert.True(t, result.Allowed, "soft enforcement should allow request")
}

func TestMemoryStoreHardEnforcementBlocks(t *testing.T) {
	t.Parallel()
	store := newTestStore(100, 1.0, HardEnforcement, nil)
	ctx := context.Background()

	err := store.Reserve(ctx, testKey(), 200, 2.0)
	require.ErrorIs(t, err, ErrBudgetExceeded)
}

// --- helpers ---

func testKey() BudgetKey {
	id := uuid.New()
	return BudgetKey{ConsumerID: &id}
}
