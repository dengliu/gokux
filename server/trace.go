package server

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// traceProvider holds the OTel TracerProvider for distributed tracing.
type traceProvider struct {
	provider *sdktrace.TracerProvider
}

// newTraceProvider creates a TracerProvider with the given SpanExporter.
// If exporter is nil, a noop provider is used (no tracing overhead).
func newTraceProvider(exporter sdktrace.SpanExporter) *traceProvider {
	var opts []sdktrace.TracerProviderOption
	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(opts...)

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
func (t *traceProvider) TracerProvider() *sdktrace.TracerProvider {
	return t.provider
}

// Tracer returns a Tracer scoped to the given instrumentation name.
func (t *traceProvider) Tracer(name string) trace.Tracer {
	return t.provider.Tracer(name)
}

// Shutdown flushes pending spans and stops the provider.
func (t *traceProvider) Shutdown(ctx context.Context) error {
	return t.provider.Shutdown(ctx)
}
