package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

func TestConsumerContext(t *testing.T) {
	t.Parallel()

	t.Run("SetConsumer and GetConsumer round-trip", func(t *testing.T) {
		t.Parallel()
		consumer := &domain.Consumer{
			ID:        uuid.New(),
			Name:      "test-consumer",
			Metadata:  json.RawMessage(`{}`),
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		ctx := SetConsumer(context.Background(), consumer)
		got := GetConsumer(ctx)
		require.NotNil(t, got)
		assert.Equal(t, consumer.ID, got.ID)
		assert.Equal(t, consumer.Name, got.Name)
	})

	t.Run("GetConsumer on empty context returns nil", func(t *testing.T) {
		t.Parallel()
		got := GetConsumer(context.Background())
		assert.Nil(t, got)
	})

	t.Run("GetConsumer with wrong type returns nil", func(t *testing.T) {
		t.Parallel()
		// Simulate wrong value type in context -- should not panic
		ctx := context.WithValue(context.Background(), ctxKeyConsumer{}, "not-a-consumer")
		got := GetConsumer(ctx)
		assert.Nil(t, got)
	})
}
