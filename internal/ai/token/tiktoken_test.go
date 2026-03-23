package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cto-venkat/apiloom-ce/internal/ai"
)

func TestTiktokenCount_HelloWorld(t *testing.T) {
	t.Parallel()

	// "Hello, world!" with cl100k_base is known to be ~4 tokens.
	count, err := tiktokenCount("Hello, world!")
	require.NoError(t, err)
	assert.Greater(t, count, int64(0))
	// Reasonable range: 3-6 tokens for "Hello, world!"
	assert.InDelta(t, 4, count, 3)
}

func TestTiktokenCount_Empty(t *testing.T) {
	t.Parallel()

	count, err := tiktokenCount("")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestTiktokenCount_MultiByte(t *testing.T) {
	t.Parallel()

	// Multi-byte UTF-8 text should not panic or error.
	count, err := tiktokenCount("Bonjour le monde! 日本語テスト")
	require.NoError(t, err)
	assert.Greater(t, count, int64(0))
}

func TestTiktokenCountMessages(t *testing.T) {
	t.Parallel()

	messages := []ai.ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
	}

	count, err := tiktokenCountMessages(messages)
	require.NoError(t, err)
	// 2 messages * 4 overhead each = 8 overhead + text tokens + 2 reply priming
	assert.Greater(t, count, int64(10))
}
