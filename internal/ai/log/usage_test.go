package ailog

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageEntry(t *testing.T) {
	t.Parallel()

	t.Run("all fields populated", func(t *testing.T) {
		t.Parallel()
		cid := uuid.New()
		rid := uuid.New()
		entry := UsageEntry{
			ConsumerID:       &cid,
			RouteID:          &rid,
			Provider:         "openai",
			Model:            "gpt-4",
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			CostUSD:          0.0045,
			LatencyMs:        320,
			IsStreaming:       false,
		}
		assert.Equal(t, "openai", entry.Provider)
		assert.Equal(t, int64(150), entry.TotalTokens)
	})
}

// mockPool captures INSERT calls for testing without a real database.
type mockPool struct {
	execCalled bool
	execCtx    context.Context
	execSQL    string
	execArgs   []any
	execErr    error
}

func (m *mockPool) Exec(ctx context.Context, sql string, args ...any) error {
	m.execCalled = true
	m.execCtx = ctx
	m.execSQL = sql
	m.execArgs = args
	return m.execErr
}

func TestPostgresUsageLogger(t *testing.T) {
	t.Parallel()

	t.Run("LogUsage inserts with parameterized SQL", func(t *testing.T) {
		t.Parallel()
		pool := &mockPool{}
		logger := NewPostgresUsageLogger(pool)

		cid := uuid.New()
		rid := uuid.New()
		entry := UsageEntry{
			ConsumerID:       &cid,
			RouteID:          &rid,
			Provider:         "openai",
			Model:            "gpt-4",
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			CostUSD:          0.0045,
			LatencyMs:        320,
			IsStreaming:       false,
		}

		err := logger.LogUsage(context.Background(), entry)
		require.NoError(t, err)
		assert.True(t, pool.execCalled)
		assert.Contains(t, pool.execSQL, "INSERT INTO ai_usage_log")
		assert.Contains(t, pool.execSQL, "$1")
		assert.Len(t, pool.execArgs, 10)
	})

	t.Run("LogUsage respects context timeout", func(t *testing.T) {
		t.Parallel()
		pool := &mockPool{}
		logger := NewPostgresUsageLogger(pool)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond) // ensure timeout fires

		entry := UsageEntry{
			Provider: "openai",
			Model:    "gpt-4",
		}

		err := logger.LogUsage(ctx, entry)
		require.Error(t, err)
	})

	t.Run("LogUsage propagates database errors", func(t *testing.T) {
		t.Parallel()
		pool := &mockPool{execErr: assert.AnError}
		logger := NewPostgresUsageLogger(pool)

		entry := UsageEntry{Provider: "openai", Model: "gpt-4"}
		err := logger.LogUsage(context.Background(), entry)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestNoopUsageLogger(t *testing.T) {
	t.Parallel()

	logger := &NoopUsageLogger{}
	err := logger.LogUsage(context.Background(), UsageEntry{})
	require.NoError(t, err)
}
