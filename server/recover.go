package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// recoverMiddleware catches panics from downstream handlers, records
// them on the active OTel span (so traces reflect the failure instead
// of showing a silent 500), logs the stack, and returns 500.
//
// Must be installed AFTER otelecho so that trace.SpanFromContext
// resolves to the request's span. Installed before this middleware,
// panics would escape the span and the trace would show no error.
func recoverMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			defer func() {
				r := recover()
				if r == nil {
					return
				}

				stack := debug.Stack()

				var panicErr error
				if e, ok := r.(error); ok {
					panicErr = e
				} else {
					panicErr = fmt.Errorf("panic: %v", r)
				}

				ctx := c.Request().Context()
				span := trace.SpanFromContext(ctx)
				span.RecordError(panicErr, trace.WithStackTrace(true))
				span.SetStatus(codes.Error, "handler panic")

				logger.ErrorContext(ctx, "handler panicked",
					"error", panicErr.Error(),
					"path", c.Path(),
					"method", c.Request().Method,
					"stack", string(stack),
				)

				err = c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "internal server error",
				})
			}()

			return next(c)
		}
	}
}
