//go:build integration

package config

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
	"github.com/mindfulcto-labs/apiloom-ce/internal/testutil"
)

func setupConsumerStore(t *testing.T) (*ConfigStore, context.Context) {
	t.Helper()
	pool, _ := testutil.SetupTestDB(t, Migrations, "migrations")
	ctx := ContextWithOrg(context.Background(), defaultOrgID)
	return NewConfigStore(pool), ctx
}

func TestCreateConsumer(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	t.Run("happy path creates consumer with generated ID", func(t *testing.T) {
		consumer, err := store.CreateConsumer(ctx, "test-consumer", json.RawMessage(`{"tier":"free"}`))
		require.NoError(t, err)
		require.NotNil(t, consumer)

		assert.NotEqual(t, consumer.ID.String(), "00000000-0000-0000-0000-000000000000")
		assert.Equal(t, "test-consumer", consumer.Name)
		assert.Equal(t, defaultOrgID, consumer.OrganizationID)
		assert.True(t, consumer.IsActive)
		assert.False(t, consumer.CreatedAt.IsZero())
		assert.False(t, consumer.UpdatedAt.IsZero())
	})

	t.Run("duplicate name returns ErrAlreadyExists", func(t *testing.T) {
		_, err := store.CreateConsumer(ctx, "duplicate-consumer", json.RawMessage(`{}`))
		require.NoError(t, err)

		_, err = store.CreateConsumer(ctx, "duplicate-consumer", json.RawMessage(`{}`))
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})
}

func TestGetConsumerByID(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	t.Run("returns consumer by ID", func(t *testing.T) {
		created, err := store.CreateConsumer(ctx, "get-test", json.RawMessage(`{}`))
		require.NoError(t, err)

		got, err := store.GetConsumer(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "get-test", got.Name)
	})

	t.Run("missing consumer returns ErrNotFound", func(t *testing.T) {
		_, err := store.GetConsumer(ctx, domain.Consumer{}.ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestListConsumers(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	_, err := store.CreateConsumer(ctx, "alpha-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)
	_, err = store.CreateConsumer(ctx, "beta-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	consumers, err := store.ListConsumers(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(consumers), 2)

	// Verify order by name.
	for i := 1; i < len(consumers); i++ {
		assert.LessOrEqual(t, consumers[i-1].Name, consumers[i].Name)
	}
}

func TestCreateAPIKey(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	consumer, err := store.CreateConsumer(ctx, "apikey-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	t.Run("generates key with bcrypt hash", func(t *testing.T) {
		rawKey, apiKey, err := store.CreateAPIKey(ctx, consumer.ID, nil)
		require.NoError(t, err)
		require.NotEmpty(t, rawKey)
		require.NotNil(t, apiKey)

		// Raw key is 64 chars (32 bytes hex-encoded).
		assert.Len(t, rawKey, 64)

		// Prefix is first 8 chars.
		assert.Equal(t, rawKey[:8], apiKey.KeyPrefix)

		// Verify bcrypt comparison works.
		err = bcrypt.CompareHashAndPassword([]byte(apiKey.KeyHash), []byte(rawKey))
		assert.NoError(t, err)

		assert.Equal(t, consumer.ID, apiKey.ConsumerID)
		assert.Equal(t, defaultOrgID, apiKey.OrganizationID)
		assert.True(t, apiKey.IsActive)
	})
}

func TestGetAPIKeysByPrefix(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	consumer, err := store.CreateConsumer(ctx, "prefix-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	rawKey, _, err := store.CreateAPIKey(ctx, consumer.ID, nil)
	require.NoError(t, err)

	prefix := rawKey[:8]
	keys, err := store.GetAPIKeysByPrefix(ctx, prefix)
	require.NoError(t, err)
	assert.NotEmpty(t, keys)

	// Verify we can find a matching key via bcrypt.
	found := false
	for _, k := range keys {
		if bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(rawKey)) == nil {
			found = true
			break
		}
	}
	assert.True(t, found, "should find matching key by prefix + bcrypt")
}

func TestRevokeAPIKey(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	consumer, err := store.CreateConsumer(ctx, "revoke-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	rawKey, apiKey, err := store.CreateAPIKey(ctx, consumer.ID, nil)
	require.NoError(t, err)

	err = store.RevokeAPIKey(ctx, apiKey.ID)
	require.NoError(t, err)

	// Lookup by prefix should skip revoked keys.
	keys, err := store.GetAPIKeysByPrefix(ctx, rawKey[:8])
	require.NoError(t, err)

	for _, k := range keys {
		assert.NotEqual(t, apiKey.ID, k.ID, "revoked key should not appear in active lookups")
	}
}

func TestRotateAPIKey(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	consumer, err := store.CreateConsumer(ctx, "rotate-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	_, oldKey, err := store.CreateAPIKey(ctx, consumer.ID, nil)
	require.NoError(t, err)

	gracePeriod := 24 * time.Hour
	newRawKey, newKey, err := store.RotateAPIKey(ctx, oldKey.ID, gracePeriod)
	require.NoError(t, err)
	require.NotEmpty(t, newRawKey)
	require.NotNil(t, newKey)

	// New key belongs to same consumer.
	assert.Equal(t, consumer.ID, newKey.ConsumerID)
	assert.True(t, newKey.IsActive)

	// New key is different from old key.
	assert.NotEqual(t, oldKey.ID, newKey.ID)

	// Verify new key bcrypt works.
	err = bcrypt.CompareHashAndPassword([]byte(newKey.KeyHash), []byte(newRawKey))
	assert.NoError(t, err)
}

func TestUpdateAPIKeyLastUsed(t *testing.T) {
	store, ctx := setupConsumerStore(t)

	consumer, err := store.CreateConsumer(ctx, "lastused-consumer", json.RawMessage(`{}`))
	require.NoError(t, err)

	_, apiKey, err := store.CreateAPIKey(ctx, consumer.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, apiKey.LastUsedAt)

	err = store.UpdateAPIKeyLastUsed(ctx, apiKey.ID)
	require.NoError(t, err)
}
