package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

func TestNewTraceProvider_NilExporter(t *testing.T) {
	tp := newTraceProvider(nil)

	assert.Nil(t, tp.provider, "provider should be nil for noop mode")
}

func TestNewTraceProvider_WithExporter(t *testing.T) {
	exporter, err := stdouttrace.New()
	require.NoError(t, err)

	tp := newTraceProvider(exporter)

	assert.NotNil(t, tp.provider, "provider should be created when exporter is provided")
}

func TestTraceProvider_Tracer_Noop(t *testing.T) {
	tp := newTraceProvider(nil)

	tracer := tp.Tracer("test")
	assert.NotNil(t, tracer, "noop tracer should be returned when provider is nil")

	// Noop tracer should still work — Start returns a valid (noop) span.
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)
	assert.False(t, span.SpanContext().IsValid(), "noop span should not have a valid SpanContext")
}

func TestTraceProvider_Tracer_WithExporter(t *testing.T) {
	exporter, err := stdouttrace.New()
	require.NoError(t, err)

	tp := newTraceProvider(exporter)

	tracer := tp.Tracer("test")
	assert.NotNil(t, tracer, "real tracer should be returned when exporter is provided")
}

func TestTraceProvider_Shutdown_Noop(t *testing.T) {
	tp := newTraceProvider(nil)

	err := tp.Shutdown(context.Background())
	assert.NoError(t, err, "shutdown should be a no-op when provider is nil")
}

func TestTraceProvider_Shutdown_WithExporter(t *testing.T) {
	exporter, err := stdouttrace.New()
	require.NoError(t, err)

	tp := newTraceProvider(exporter)

	err = tp.Shutdown(context.Background())
	assert.NoError(t, err, "shutdown should flush and stop cleanly")
}

func TestTraceProvider_TracerProvider_Noop(t *testing.T) {
	tp := newTraceProvider(nil)

	assert.Nil(t, tp.TracerProvider(), "TracerProvider should return nil in noop mode")
}

func TestTraceProvider_TracerProvider_WithExporter(t *testing.T) {
	exporter, err := stdouttrace.New()
	require.NoError(t, err)

	tp := newTraceProvider(exporter)

	assert.NotNil(t, tp.TracerProvider(), "TracerProvider should return the SDK provider")
}
