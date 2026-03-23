package ailog

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"
)

// LogLevel controls the verbosity of AI request/response logging.
type LogLevel int

const (
	// Off disables all AI logging.
	Off LogLevel = iota
	// MetadataOnly logs token counts, model, latency, consumer (no prompt/response content).
	MetadataOnly
	// Full logs metadata plus redacted prompt and response content.
	Full
)

// Default PII redaction patterns.
var defaultPatterns = []string{
	`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`,    // email
	`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`,                              // phone
	`\b\d{3}-\d{2}-\d{4}\b`,                                      // SSN
	`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`,                // credit card
}

// API key patterns that must never be logged per security.md.
var apiKeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9\-._~+/]+=*`),
}

// LogEntry holds all data for a single AI request log record.
type LogEntry struct {
	Timestamp        time.Time
	ConsumerID       string
	RouteID          string
	Provider         string
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	CostUSD          float64
	LatencyMs        int64
	IsStreaming      bool
	Prompt           string // optional, only used in Full mode
	Response         string // optional, only used in Full mode
}

// AILogger logs AI requests with configurable verbosity and PII redaction.
type AILogger struct {
	logger         *slog.Logger
	redactPatterns []*regexp.Regexp
}

// NewAILogger creates an AILogger with the given slog logger and optional
// additional regex patterns for PII redaction. Default patterns (email, phone,
// SSN, credit card) are always included. Pass nil for no additional patterns.
func NewAILogger(logger *slog.Logger, additionalPatterns []string) (*AILogger, error) {
	allPatterns := make([]string, 0, len(defaultPatterns)+len(additionalPatterns))
	allPatterns = append(allPatterns, defaultPatterns...)
	allPatterns = append(allPatterns, additionalPatterns...)

	compiled := make([]*regexp.Regexp, 0, len(allPatterns))
	for _, p := range allPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("compiling redaction pattern %q: %w", p, err)
		}
		compiled = append(compiled, re)
	}

	return &AILogger{
		logger:         logger,
		redactPatterns: compiled,
	}, nil
}

// Redact applies all PII redaction patterns to the input text.
func (l *AILogger) Redact(text string) string {
	for _, re := range l.redactPatterns {
		text = re.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}

// StripAPIKeys removes API key patterns from text per security.md.
// Provider API keys must never appear in logs.
func (l *AILogger) StripAPIKeys(text string) string {
	for _, re := range apiKeyPatterns {
		text = re.ReplaceAllString(text, "[API_KEY_REDACTED]")
	}
	return text
}

// Log records an AI request log entry at the specified level.
// Off: no logging. MetadataOnly: metadata without content. Full: all fields with PII redaction.
func (l *AILogger) Log(_ context.Context, level LogLevel, entry LogEntry) {
	if level == Off {
		return
	}

	attrs := []slog.Attr{
		slog.String("consumer_id", entry.ConsumerID),
		slog.String("route_id", entry.RouteID),
		slog.String("provider", entry.Provider),
		slog.String("model", entry.Model),
		slog.Int64("prompt_tokens", entry.PromptTokens),
		slog.Int64("completion_tokens", entry.CompletionTokens),
		slog.Float64("cost_usd", entry.CostUSD),
		slog.Int64("latency_ms", entry.LatencyMs),
		slog.Bool("is_streaming", entry.IsStreaming),
	}

	if level == Full {
		// Apply PII redaction and API key stripping before logging.
		prompt := l.StripAPIKeys(l.Redact(entry.Prompt))
		response := l.StripAPIKeys(l.Redact(entry.Response))
		attrs = append(attrs,
			slog.String("prompt", prompt),
			slog.String("response", response),
		)
	}

	// Convert to []any for LogAttrs.
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, "ai_request", attrs...)
}
