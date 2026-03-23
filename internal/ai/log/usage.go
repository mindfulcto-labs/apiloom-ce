// Package ailog provides AI request logging with PII redaction and usage tracking.
package ailog

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UsageEntry represents a single AI usage log record.
type UsageEntry struct {
	ConsumerID       *uuid.UUID
	RouteID          *uuid.UUID
	Provider         string
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CostUSD          float64
	LatencyMs        int64
	IsStreaming      bool
}

// UsageLogger records AI usage to a persistent store.
type UsageLogger interface {
	LogUsage(ctx context.Context, entry UsageEntry) error
}

// DBExecer abstracts database exec for testing without pgxpool dependency.
type DBExecer interface {
	Exec(ctx context.Context, sql string, args ...any) error
}

// PostgresUsageLogger writes usage records to ai_usage_log via parameterized SQL.
type PostgresUsageLogger struct {
	db DBExecer
}

// NewPostgresUsageLogger creates a UsageLogger backed by Postgres.
func NewPostgresUsageLogger(db DBExecer) *PostgresUsageLogger {
	return &PostgresUsageLogger{db: db}
}

// LogUsage inserts a usage record with a 5-second context timeout.
func (l *PostgresUsageLogger) LogUsage(ctx context.Context, entry UsageEntry) error {
	insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Check if parent context is already expired.
	if err := insertCtx.Err(); err != nil {
		return fmt.Errorf("logging usage: %w", err)
	}

	const query = `INSERT INTO ai_usage_log
		(consumer_id, route_id, provider, model, prompt_tokens, completion_tokens, total_tokens, cost_usd, latency_ms, is_streaming)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	if err := l.db.Exec(insertCtx, query,
		entry.ConsumerID,
		entry.RouteID,
		entry.Provider,
		entry.Model,
		entry.PromptTokens,
		entry.CompletionTokens,
		entry.TotalTokens,
		entry.CostUSD,
		entry.LatencyMs,
		entry.IsStreaming,
	); err != nil {
		return fmt.Errorf("inserting ai_usage_log: %w", err)
	}

	return nil
}

// NoopUsageLogger discards all usage records (for testing or when logging is disabled).
type NoopUsageLogger struct{}

// LogUsage is a no-op.
func (l *NoopUsageLogger) LogUsage(_ context.Context, _ UsageEntry) error {
	return nil
}
