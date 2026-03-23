package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		statusCode     int
		requestID      string
		wantFields     []string
		wantLevel      slog.Level
	}{
		{
			name:       "logs basic request fields",
			method:     http.MethodGet,
			path:       "/api/v1/test",
			statusCode: http.StatusOK,
			wantFields: []string{"method", "path", "status", "latency_ms", "request_id"},
			wantLevel:  slog.LevelDebug,
		},
		{
			name:       "logs POST request",
			method:     http.MethodPost,
			path:       "/api/v1/users",
			statusCode: http.StatusCreated,
			wantFields: []string{"method", "path", "status"},
			wantLevel:  slog.LevelDebug,
		},
		{
			name:       "preserves X-Request-ID header",
			method:     http.MethodGet,
			path:       "/test",
			statusCode: http.StatusOK,
			requestID:  "custom-request-id-123",
			wantFields: []string{"request_id"},
			wantLevel:  slog.LevelDebug,
		},
		{
			name:       "logs at configured level",
			method:     http.MethodGet,
			path:       "/test",
			statusCode: http.StatusOK,
			wantLevel:  slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			cfg := LoggingConfig{Level: tt.wantLevel}

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte("ok"))
			})

			handler := LoggingMiddleware(logger, cfg)(inner)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.statusCode, rec.Code)

			// Parse the log output.
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err, "log output should be valid JSON: %s", buf.String())

			for _, field := range tt.wantFields {
				assert.Contains(t, logEntry, field, "log should contain field %q", field)
			}

			// Verify specific field values.
			assert.Equal(t, tt.method, logEntry["method"])
			assert.Equal(t, tt.path, logEntry["path"])
			assert.Equal(t, float64(tt.statusCode), logEntry["status"])

			if tt.requestID != "" {
				assert.Equal(t, tt.requestID, logEntry["request_id"])
			} else {
				// Should have auto-generated a UUID request_id.
				rid, ok := logEntry["request_id"].(string)
				require.True(t, ok, "request_id should be a string")
				_, err := uuid.Parse(rid)
				assert.NoError(t, err, "auto-generated request_id should be valid UUID")
			}

			// Verify latency_ms is present and positive.
			latency, ok := logEntry["latency_ms"].(float64)
			require.True(t, ok, "latency_ms should be a number")
			assert.GreaterOrEqual(t, latency, float64(0))
		})
	}
}

func TestLoggingMiddleware_SensitiveHeaderRedaction(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := LoggingMiddleware(logger, LoggingConfig{Level: slog.LevelDebug})(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("X-Api-Key", "key-12345678")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()

	// Sensitive header values must NEVER appear in log output.
	assert.NotContains(t, logOutput, "super-secret-token")
	assert.NotContains(t, logOutput, "session=abc123")
	assert.NotContains(t, logOutput, "key-12345678")

	// Non-sensitive headers can appear.
	assert.Contains(t, logOutput, "application/json")
}

func TestLoggingMiddleware_TraceIDFromSpan(t *testing.T) {
	// Set up OTel tracing so we get a real trace ID.
	exp := newTestExporter()
	processor := sdktrace.NewSimpleSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chain: tracing middleware -> logging middleware -> handler
	handler := TracingMiddleware(LoggingMiddleware(logger, LoggingConfig{Level: slog.LevelDebug})(inner))

	req := httptest.NewRequest(http.MethodGet, "/traced", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// trace_id should be present and valid (32 hex chars).
	traceID, ok := logEntry["trace_id"].(string)
	require.True(t, ok, "trace_id should be a string")
	assert.Len(t, traceID, 32, "trace_id should be 32 hex characters")
	assert.NotEqual(t, "00000000000000000000000000000000", traceID)
}

func TestLoggingMiddleware_NoTraceID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := LoggingMiddleware(logger, LoggingConfig{Level: slog.LevelDebug})(inner)

	req := httptest.NewRequest(http.MethodGet, "/no-trace", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// trace_id should be empty string or absent when no tracing context.
	traceID, _ := logEntry["trace_id"].(string)
	assert.Empty(t, traceID, "trace_id should be empty when no tracing context")
}
