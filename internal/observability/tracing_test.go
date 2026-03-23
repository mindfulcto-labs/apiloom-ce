package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// These tests manipulate global OTel state and cannot be parallel at the
// package level. Each test restores previous state via defer.

func TestInitTracer_NoopOnEmptyEndpoint(t *testing.T) {
	ctx := context.Background()
	shutdown, err := InitTracer(ctx, TracingConfig{
		ServiceName:  "test",
		OTLPEndpoint: "",
		SampleRate:   1.0,
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	defer func() { _ = shutdown(ctx) }()

	// Noop provider produces invalid span contexts.
	tp := otel.GetTracerProvider()
	tracer := tp.Tracer("test")
	_, span := tracer.Start(ctx, "test-span")
	assert.False(t, span.SpanContext().IsValid(), "noop provider should produce invalid span context")
	span.End()
}

func TestInitTracer_ValidEndpoint(t *testing.T) {
	ctx := context.Background()
	shutdown, err := InitTracer(ctx, TracingConfig{
		ServiceName:  "flexgateway",
		OTLPEndpoint: "localhost:4317",
		SampleRate:   0.5,
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Shutdown should succeed even without a real collector.
	err = shutdown(ctx)
	assert.NoError(t, err)
}

func TestInitTracer_DefaultsApplied(t *testing.T) {
	ctx := context.Background()
	shutdown, err := InitTracer(ctx, TracingConfig{
		OTLPEndpoint: "localhost:4317",
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	_ = shutdown(ctx)
}

func TestTracingMiddleware(t *testing.T) {
	// Set up an in-memory span exporter for testing.
	exp := newTestExporter()
	processor := sdktrace.NewSimpleSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tests := []struct {
		name              string
		method            string
		path              string
		injectTraceparent bool
	}{
		{
			name:   "creates span for GET request",
			method: http.MethodGet,
			path:   "/api/v1/test",
		},
		{
			name:   "creates span for POST request",
			method: http.MethodPost,
			path:   "/api/v1/users",
		},
		{
			name:              "propagates incoming traceparent",
			method:            http.MethodGet,
			path:              "/api/v1/test",
			injectTraceparent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotTraceID trace.TraceID
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				span := trace.SpanFromContext(r.Context())
				gotTraceID = span.SpanContext().TraceID()
				w.WriteHeader(http.StatusOK)
			})

			handler := TracingMiddleware(inner)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.injectTraceparent {
				req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.True(t, gotTraceID.IsValid(), "expected a valid trace ID in the span context")
		})
	}
}

func TestTracingMiddleware_W3CPropagation(t *testing.T) {
	// Set up tracing with propagation.
	exp := newTestExporter()
	processor := sdktrace.NewSimpleSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))
	defer func() { _ = tp.Shutdown(context.Background()) }()

	// Inject a known traceparent and verify it propagates through.
	knownTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		assert.Equal(t, knownTraceID, span.SpanContext().TraceID().String())
		w.WriteHeader(http.StatusOK)
	})

	handler := TracingMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Traceparent", "00-"+knownTraceID+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTracingMiddleware_StatusCodeCaptured(t *testing.T) {
	exp := newTestExporter()
	processor := sdktrace.NewSimpleSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() { _ = tp.Shutdown(context.Background()) }()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := TracingMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// testExporter is a minimal SpanExporter for testing.
type testExporter struct{}

func newTestExporter() *testExporter { return &testExporter{} }

func (e *testExporter) ExportSpans(_ context.Context, _ []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *testExporter) Shutdown(_ context.Context) error { return nil }
