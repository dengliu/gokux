package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// noopPanicCounter returns an Int64Counter that records into the void —
// used by tests that don't assert on the counter but still need a valid
// instrument to satisfy recoverMiddleware's signature.
func noopPanicCounter(t *testing.T) metric.Int64Counter {
	t.Helper()
	c, err := noop.NewMeterProvider().Meter("test").Int64Counter("panic")
	require.NoError(t, err)
	return c
}

func TestRecoverMiddleware_ErrorPanic(t *testing.T) {
	e := echo.New()
	e.Use(recoverMiddleware(testLogger(), noopPanicCounter(t)))
	e.GET("/boom", func(c echo.Context) error {
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()

	// Must not let the panic escape.
	assert.NotPanics(t, func() { e.ServeHTTP(rec, req) })

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal server error")
}

func TestRecoverMiddleware_StringPanic(t *testing.T) {
	e := echo.New()
	e.Use(recoverMiddleware(testLogger(), noopPanicCounter(t)))
	e.GET("/boom", func(c echo.Context) error {
		panic("string panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() { e.ServeHTTP(rec, req) })
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecoverMiddleware_NoPanicPassesThrough(t *testing.T) {
	e := echo.New()
	e.Use(recoverMiddleware(testLogger(), noopPanicCounter(t)))
	e.GET("/ok", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

// TestRecoverMiddleware_RecordsOnSpan is the regression test for issue #7:
// when a handler panics, the active OTel span must be marked Error and
// the panic must be recorded as an event. Installed after otelecho so
// trace.SpanFromContext resolves to the request's span.
func TestRecoverMiddleware_RecordsOnSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	e := echo.New()
	e.Use(otelecho.Middleware("test", otelecho.WithTracerProvider(tp)))
	e.Use(recoverMiddleware(testLogger(), noopPanicCounter(t)))
	e.GET("/boom", func(c echo.Context) error {
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	assert.NotPanics(t, func() { e.ServeHTTP(rec, req) })

	spans := exporter.GetSpans()
	require.Len(t, spans, 1, "expected one span for the request")

	span := spans[0]
	assert.Equal(t, codes.Error, span.Status.Code, "panic must mark span as error")

	require.NotEmpty(t, span.Events, "span must carry the panic as an event")
	foundExceptionEvent := false
	for _, ev := range span.Events {
		if ev.Name == "exception" {
			foundExceptionEvent = true
			break
		}
	}
	assert.True(t, foundExceptionEvent, "expected an 'exception' event from RecordError")
}

// TestRecoverMiddleware_IncrementsPanicCounter is the regression test for
// issue #27: a handler panic must increment the panic counter with
// attribute where="handler" so alerts can fire.
func TestRecoverMiddleware_IncrementsPanicCounter(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	counter, err := mp.Meter("test").Int64Counter("gokux.panic")
	require.NoError(t, err)

	e := echo.New()
	e.Use(recoverMiddleware(testLogger(), counter))
	e.GET("/boom", func(c echo.Context) error {
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	assert.NotPanics(t, func() { e.ServeHTTP(rec, req) })

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	sum := findCounterSum(t, rm, "gokux.panic", attribute.String("where", "handler"))
	assert.EqualValues(t, 1, sum, "handler panic should increment gokux.panic{where=handler} by 1")
}

// findCounterSum returns the accumulated value of the named Int64 sum
// metric filtered to a data point carrying the expected attribute.
func findCounterSum(t *testing.T, rm metricdata.ResourceMetrics, name string, match attribute.KeyValue) int64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "metric %s is not an int64 sum", name)
			for _, dp := range sum.DataPoints {
				if v, ok := dp.Attributes.Value(match.Key); ok && v == match.Value {
					return dp.Value
				}
			}
		}
	}
	t.Fatalf("counter %s with attribute %s=%s not found", name, match.Key, match.Value.Emit())
	return 0
}
