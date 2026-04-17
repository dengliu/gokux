package server

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// traceProvider holds the OTel TracerProvider for distributed tracing.
type traceProvider struct {
	provider *sdktrace.TracerProvider
}

// newTraceProvider creates a TracerProvider with the given SpanExporter.
// If exporter is nil, returns a traceProvider wrapping the default noop —
// no SDK TracerProvider is created and no middleware overhead is added.
func newTraceProvider(exporter sdktrace.SpanExporter) *traceProvider {
	if exporter == nil {
		// No exporter: true noop — zero allocations, zero overhead.
		return &traceProvider{provider: nil}
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)

	// Set as global so otelecho middleware and other OTel integrations use it.
	otel.SetTracerProvider(provider)

	// Set W3C TraceContext + Baggage propagation so trace context
	// is extracted from / injected into HTTP headers.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &traceProvider{provider: provider}
}

// TracerProvider returns the underlying OTel TracerProvider.
// Returns nil if tracing is not configured (noop mode).
func (t *traceProvider) TracerProvider() *sdktrace.TracerProvider {
	return t.provider
}

// Tracer returns a Tracer scoped to the given instrumentation name.
// If tracing is not configured, returns the global noop tracer.
func (t *traceProvider) Tracer(name string) trace.Tracer {
	if t.provider == nil {
		return noop.NewTracerProvider().Tracer(name)
	}

	return t.provider.Tracer(name)
}

// Shutdown flushes pending spans and stops the provider.
// If tracing is not configured, this is a no-op.
func (t *traceProvider) Shutdown(ctx context.Context) error {
	if t.provider == nil {
		return nil
	}

	return t.provider.Shutdown(ctx)
}
