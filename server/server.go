// Package server provides the HTTP server, routing, and middleware for the gokux service.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/dengliu/gokux/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Server wraps an Echo instance with application dependencies.
type Server struct {
	Echo    *echo.Echo
	config  *config.Config
	logger  *slog.Logger
	ready   *atomic.Bool
	health  *healthHandler
	metrics *metricsProvider
	traces  *traceProvider
}

// NewServer creates a configured Echo server with all routes and middleware.
// Returns an error if the OTel metrics provider fails to initialize.
// The traceExporter is optional — pass nil for noop tracing.
// Most consumers should use gokux.New() + Init() instead of calling this directly.
func NewServer(cfg *config.Config, logger *slog.Logger, traceExporter sdktrace.SpanExporter) (*Server, error) {
	ready := &atomic.Bool{}
	ready.Store(true)

	mp, err := newMetricsProvider()
	if err != nil {
		return nil, fmt.Errorf("create metrics provider: %w", err)
	}

	tp := newTraceProvider(traceExporter)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	if traceExporter != nil {
		e.Use(otelecho.Middleware("gokux"))
	}
	e.Use(slogMiddleware(logger))
	e.Use(metricsMiddleware(mp))

	// Health check routes
	health := newHealthHandler(ready)
	e.GET("/healthz", health.Healthz)
	e.GET("/readyz", health.Readyz)

	// Metrics route
	e.GET("/metrics", metricsHandler(mp))

	return &Server{
		Echo:    e,
		config:  cfg,
		logger:  logger,
		ready:   ready,
		health:  health,
		metrics: mp,
		traces:  tp,
	}, nil
}

// Start begins listening on the configured port. This call blocks.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Server.Port)
	s.logger.Info("starting server", "addr", addr)

	return s.Echo.Start(addr)
}

// Shutdown performs a graceful shutdown: marks the service as not-ready,
// waits for in-flight requests to drain, then stops the server.
func (s *Server) Shutdown(timeout time.Duration) error {
	s.logger.Info("shutting down server")

	// Mark as not ready so readiness probe fails and
	// Kubernetes stops sending new traffic.
	s.ready.Store(false)

	// Allow time for load balancers to detect the readiness change.
	drainWait := s.config.Server.DrainWaitDuration()
	s.logger.Info("waiting for in-flight requests to drain", "drain", drainWait)
	time.Sleep(drainWait)

	// Create a context with a timeout for the shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return s.Echo.Shutdown(ctx)
}

// MeterProvider returns the OTel MeterProvider used by this server.
// Use it to create custom meters for application-specific instruments.
func (s *Server) MeterProvider() *sdkmetric.MeterProvider {
	return s.metrics.provider
}

// TracerProvider returns the OTel TracerProvider used by this server.
// Returns nil if tracing is not configured (noop mode).
// For most use cases, prefer App.Tracer(name) instead.
func (s *Server) TracerProvider() *sdktrace.TracerProvider {
	return s.traces.TracerProvider()
}

// Tracer returns an OTel Tracer scoped to the given instrumentation name.
// Safe to call even when tracing is not configured (returns noop tracer).
func (s *Server) Tracer(name string) trace.Tracer {
	return s.traces.Tracer(name)
}

// AddLivenessCheck registers a named liveness check.
// If any liveness check fails, /healthz returns 503.
func (s *Server) AddLivenessCheck(name string, check HealthCheck) {
	s.health.AddLivenessCheck(name, check)
}

// AddReadinessCheck registers a named readiness check.
// If any readiness check fails (or the server is draining), /readyz returns 503.
func (s *Server) AddReadinessCheck(name string, check HealthCheck) {
	s.health.AddReadinessCheck(name, check)
}

// slogMiddleware returns an Echo middleware that logs each request using slog.
func slogMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()
			latency := time.Since(start)

			// Skip logging for health check and metrics endpoints to reduce noise.
			path := req.URL.Path
			if path == "/healthz" || path == "/readyz" || path == "/metrics" {
				return nil
			}

			logger.Info("request",
				"method", req.Method,
				"path", path,
				"status", res.Status,
				"latency", latency,
				"remote_ip", c.RealIP(),
				"user_agent", req.UserAgent(),
			)

			return nil
		}
	}
}
