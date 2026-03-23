package ailog

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogLevel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LogLevel(0), Off)
	assert.Equal(t, LogLevel(1), MetadataOnly)
	assert.Equal(t, LogLevel(2), Full)
}

func TestNewAILogger(t *testing.T) {
	t.Parallel()

	t.Run("compiles default patterns", func(t *testing.T) {
		t.Parallel()
		logger, err := NewAILogger(slog.Default(), nil)
		require.NoError(t, err)
		assert.NotNil(t, logger)
	})

	t.Run("compiles custom patterns", func(t *testing.T) {
		t.Parallel()
		logger, err := NewAILogger(slog.Default(), []string{`\bsecret\b`})
		require.NoError(t, err)
		assert.NotNil(t, logger)
	})

	t.Run("returns error for invalid regex", func(t *testing.T) {
		t.Parallel()
		_, err := NewAILogger(slog.Default(), []string{`[invalid`})
		require.Error(t, err)
	})
}

func TestRedact(t *testing.T) {
	t.Parallel()

	logger, err := NewAILogger(slog.Default(), nil)
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "redacts email",
			input: "Contact john@example.com for info",
			want:  "Contact [REDACTED] for info",
		},
		{
			name:  "redacts phone with dashes",
			input: "Call me at 555-123-4567 please",
			want:  "Call me at [REDACTED] please",
		},
		{
			name:  "redacts phone with dots",
			input: "Phone: 555.123.4567",
			want:  "Phone: [REDACTED]",
		},
		{
			name:  "redacts SSN",
			input: "SSN is 123-45-6789",
			want:  "SSN is [REDACTED]",
		},
		{
			name:  "redacts credit card with spaces",
			input: "Card: 4111 1111 1111 1111",
			want:  "Card: [REDACTED]",
		},
		{
			name:  "redacts credit card with dashes",
			input: "Card: 4111-1111-1111-1111",
			want:  "Card: [REDACTED]",
		},
		{
			name:  "no redaction needed",
			input: "Hello, this is a normal message",
			want:  "Hello, this is a normal message",
		},
		{
			name:  "multiple PII in one string",
			input: "Email: test@test.com, Phone: 555-111-2222",
			want:  "Email: [REDACTED], Phone: [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := logger.Redact(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRedactCustomPattern(t *testing.T) {
	t.Parallel()

	logger, err := NewAILogger(slog.Default(), []string{`\bsecret-\w+\b`})
	require.NoError(t, err)

	got := logger.Redact("The code is secret-abc123 here")
	assert.Equal(t, "The code is [REDACTED] here", got)
}

func TestStripAPIKeys(t *testing.T) {
	t.Parallel()

	logger, err := NewAILogger(slog.Default(), nil)
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips sk- pattern",
			input: "Key: sk-abc123def456ghi789jkl0",
			want:  "Key: [API_KEY_REDACTED]",
		},
		{
			name:  "strips Bearer token",
			input: "Token: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			want:  "Token: [API_KEY_REDACTED]",
		},
		{
			name:  "no key present",
			input: "Hello world",
			want:  "Hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := logger.StripAPIKeys(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAILoggerLog(t *testing.T) {
	t.Parallel()

	t.Run("Off mode logs nothing", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		slogger := slog.New(slog.NewTextHandler(&buf, nil))
		logger, err := NewAILogger(slogger, nil)
		require.NoError(t, err)

		entry := LogEntry{
			ConsumerID: "consumer-1",
			Provider:   "openai",
			Model:      "gpt-4",
			Prompt:     "What is your SSN? Mine is 123-45-6789",
			Response:   "I cannot help with that",
		}

		logger.Log(context.Background(), Off, entry)
		assert.Empty(t, buf.String())
	})

	t.Run("MetadataOnly logs metadata without prompt/response", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		slogger := slog.New(slog.NewTextHandler(&buf, nil))
		logger, err := NewAILogger(slogger, nil)
		require.NoError(t, err)

		entry := LogEntry{
			ConsumerID:       "consumer-1",
			Provider:         "openai",
			Model:            "gpt-4",
			PromptTokens:     10,
			CompletionTokens: 5,
			LatencyMs:        250,
			Prompt:           "secret prompt",
			Response:         "secret response",
		}

		logger.Log(context.Background(), MetadataOnly, entry)
		output := buf.String()
		assert.Contains(t, output, "openai")
		assert.Contains(t, output, "gpt-4")
		assert.NotContains(t, output, "secret prompt")
		assert.NotContains(t, output, "secret response")
	})

	t.Run("Full mode logs redacted prompt and response", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		slogger := slog.New(slog.NewTextHandler(&buf, nil))
		logger, err := NewAILogger(slogger, nil)
		require.NoError(t, err)

		entry := LogEntry{
			ConsumerID:       "consumer-1",
			Provider:         "openai",
			Model:            "gpt-4",
			PromptTokens:     10,
			CompletionTokens: 5,
			LatencyMs:        250,
			Prompt:           "My email is user@example.com",
			Response:         "I see your email user@example.com",
		}

		logger.Log(context.Background(), Full, entry)
		output := buf.String()
		assert.Contains(t, output, "[REDACTED]")
		assert.NotContains(t, output, "user@example.com")
		assert.Contains(t, output, "openai")
	})

	t.Run("Full mode strips API keys from content", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		slogger := slog.New(slog.NewTextHandler(&buf, nil))
		logger, err := NewAILogger(slogger, nil)
		require.NoError(t, err)

		entry := LogEntry{
			ConsumerID: "consumer-1",
			Provider:   "openai",
			Model:      "gpt-4",
			Prompt:     "Use key sk-abc123def456ghi789jkl012 for access",
			Response:   "OK",
		}

		logger.Log(context.Background(), Full, entry)
		output := buf.String()
		assert.NotContains(t, output, "sk-abc123")
		assert.Contains(t, output, "[API_KEY_REDACTED]")
	})
}
