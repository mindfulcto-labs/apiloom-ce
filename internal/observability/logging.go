package observability

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// sensitiveHeaders lists headers whose values must never appear in logs.
// Keys are canonicalized (http.CanonicalHeaderKey format).
var sensitiveHeaders = map[string]bool{
	"Authorization": true,
	"Cookie":        true,
	"X-Api-Key":     true,
}

// LoggingConfig configures the logging middleware.
type LoggingConfig struct {
	// Level is the log level for per-request logging. Default: slog.LevelDebug.
	// Use debug for per-request logs (avoids log flood at 10K+ RPS).
	Level slog.Level
}

// LoggingMiddleware returns HTTP middleware that logs structured JSON for each
// request. Sensitive headers are redacted per security.md.
//
// Log fields: trace_id, request_id, method, path, status, latency_ms,
// consumer_id (when auth context present), and sanitized headers at debug level.
func LoggingMiddleware(logger *slog.Logger, cfg LoggingConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extract or generate request ID.
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Wrap response writer to capture status code.
			sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			latencyMs := float64(duration.Nanoseconds()) / 1e6

			// Extract trace ID from span context (empty if no tracing).
			var traceID string
			spanCtx := trace.SpanFromContext(r.Context()).SpanContext()
			if spanCtx.HasTraceID() {
				traceID = spanCtx.TraceID().String()
			}

			// Build log attributes.
			attrs := []slog.Attr{
				slog.String("trace_id", traceID),
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.statusCode),
				slog.Float64("latency_ms", latencyMs),
			}

			// Include sanitized headers at debug level.
			if cfg.Level <= slog.LevelDebug {
				attrs = append(attrs, slog.Any("headers", sanitizeHeaders(r.Header)))
			}

			logger.LogAttrs(r.Context(), cfg.Level, "request",
				attrs...,
			)
		})
	}
}

// sanitizeHeaders returns a copy of the headers with sensitive values redacted.
func sanitizeHeaders(h http.Header) map[string]string {
	result := make(map[string]string, len(h))
	for k, v := range h {
		canonical := http.CanonicalHeaderKey(k)
		if sensitiveHeaders[canonical] {
			result[canonical] = "[REDACTED]"
		} else {
			result[canonical] = strings.Join(v, ", ")
		}
	}
	return result
}
