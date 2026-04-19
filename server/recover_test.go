package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecoverMiddleware_ErrorPanic(t *testing.T) {
	e := echo.New()
	e.Use(recoverMiddleware(testLogger()))
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
	e.Use(recoverMiddleware(testLogger()))
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
	e.Use(recoverMiddleware(testLogger()))
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
	e.Use(recoverMiddleware(testLogger()))
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
