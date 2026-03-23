// Package observability provides OTel tracing, Prometheus metrics,
// and structured logging middleware for the FlexGateway data plane.
package observability

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracingConfig configures the OTel tracing provider.
type TracingConfig struct {
	// ServiceName is the service.name resource attribute. Defaults to "flexgateway".
	ServiceName string

	// OTLPEndpoint is the gRPC endpoint for the OTLP exporter.
	// If empty, a noop provider is used (no traces exported).
	OTLPEndpoint string

	// SampleRate is the probability-based sampling rate (0.0 to 1.0).
	// Defaults to 1.0 (always sample).
	SampleRate float64

	// ServiceVersion is the service.version resource attribute.
	ServiceVersion string
}

// InitTracer initializes the OTel TracerProvider with an OTLP gRPC exporter.
// If OTLPEndpoint is empty, it sets a noop provider and returns a noop shutdown.
// The returned shutdown function must be called during graceful shutdown.
func InitTracer(ctx context.Context, cfg TracingConfig) (shutdown func(context.Context) error, err error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "flexgateway"
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 1.0
	}

	// Set W3C propagator regardless of exporter configuration.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// No endpoint: fall back to noop provider.
	if cfg.OTLPEndpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(_ context.Context) error { return nil }, nil
	}

	// Create OTLP gRPC exporter.
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	// Build resource with service metadata.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	// Create TracerProvider with batch span processor and ratio sampler.
	sampler := sdktrace.TraceIDRatioBased(cfg.SampleRate)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// ShutdownTracer is an alias kept for readability in shutdown sequences.
// Use the function returned by InitTracer directly.
var ShutdownTracer = func(ctx context.Context, shutdownFn func(context.Context) error) error {
	if shutdownFn == nil {
		return nil
	}
	return shutdownFn(ctx)
}

// TracingMiddleware creates a span per HTTP request with W3C context propagation.
// Span attributes follow OpenTelemetry semantic conventions.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := otel.GetTracerProvider().Tracer("flexgateway")
		prop := otel.GetTextMapPropagator()

		// Extract incoming trace context from headers.
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Determine span name from method + path.
		spanName := r.Method + " " + r.URL.Path

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				attribute.String("url.path", r.URL.Path),
			),
		)
		defer span.End()

		// Wrap response writer to capture status code.
		sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Inject trace context into response headers for downstream propagation.
		prop.Inject(ctx, propagation.HeaderCarrier(w.Header()))

		next.ServeHTTP(sw, r.WithContext(ctx))

		// Set status code attribute after handler completes.
		span.SetAttributes(
			semconv.HTTPResponseStatusCode(sw.statusCode),
		)
	})
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// statusCodeStr returns the status code as a string for metric labels.
func statusCodeStr(code int) string {
	return strconv.Itoa(code)
}
