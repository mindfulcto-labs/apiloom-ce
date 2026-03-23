package controlplane

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeCursor(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	cursor := encodeCursor(ts, id)
	assert.NotEmpty(t, cursor)

	decodedTime, decodedID, err := decodeCursor(cursor)
	require.NoError(t, err)
	assert.True(t, ts.Equal(decodedTime), "time should round-trip: got %v want %v", decodedTime, ts)
	assert.Equal(t, id, decodedID)
}

func TestDecodeCursor_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "not base64", cursor: "!!!notbase64!!!"},
		{name: "missing separator", cursor: "dGVzdA=="},
		{name: "invalid time", cursor: encodeTestCursor("notadate", uuid.New().String())},
		{name: "invalid uuid", cursor: encodeTestCursor(time.Now().Format(time.RFC3339Nano), "not-a-uuid")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := decodeCursor(tt.cursor)
			assert.Error(t, err)
		})
	}
}

func TestDecodeCursor_Empty(t *testing.T) {
	t.Parallel()

	ts, id, err := decodeCursor("")
	require.NoError(t, err)
	assert.True(t, ts.IsZero())
	assert.Equal(t, uuid.Nil, id)
}

func TestBuildPagedResponse(t *testing.T) {
	t.Parallel()

	t.Run("fewer than limit", func(t *testing.T) {
		t.Parallel()
		items := []string{"a", "b"}
		resp := buildPagedResponse(items, 5, func(s string) (time.Time, uuid.UUID) {
			return time.Now(), uuid.New()
		})
		assert.Len(t, resp.Data, 2)
		assert.False(t, resp.HasMore)
		assert.Empty(t, resp.NextCursor)
	})

	t.Run("more than limit means has_more", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		// Simulate limit+1 results where limit=2
		items := []string{"a", "b", "c"}
		resp := buildPagedResponse(items, 2, func(s string) (time.Time, uuid.UUID) {
			return ts, id
		})
		assert.Len(t, resp.Data, 2)
		assert.True(t, resp.HasMore)
		assert.NotEmpty(t, resp.NextCursor)
	})
}
