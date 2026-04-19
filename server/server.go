// Package server provides the HTTP server, routing, and middleware for the gokux service.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/dengliu/gokux/config"
	"github.com/labstack/echo/v4"
	slogecho "github.com/samber/slog-echo"
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
	ready := &atomic.Bool{} // defaults to false; App.Run sets it to true after bootstrap

	mp, err := newMetricsProvider()
	if err != nil {
		return nil, fmt.Errorf("create metrics provider: %w", err)
	}

	tp := newTraceProvider(traceExporter)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware — order matters:
	//   1. otelecho creates a span for every request
	//   2. Recover catches panics inside the span so we can record the error
	//   3. slogecho logs the request
	//   4. metricsMiddleware records duration/count
	if traceExporter != nil {
		e.Use(otelecho.Middleware("gokux"))
	}
	e.Use(recoverMiddleware(logger))
	e.Use(slogecho.NewWithConfig(logger, slogecho.Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
		WithUserAgent:    true,
		WithTraceID:      traceExporter != nil,
		WithSpanID:       traceExporter != nil,
		Filters: []slogecho.Filter{
			slogecho.IgnorePath("/healthz", "/readyz", "/metrics"),
		},
	}))
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
// waits for in-flight requests to drain, then stops the server and
// flushes the OTel metrics and trace providers.
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

	var errs []error

	if err := s.Echo.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("echo shutdown: %w", err))
	}

	// Flush pending metrics and spans before the process exits.
	if err := s.metrics.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("metrics provider shutdown: %w", err))
	}
	if err := s.traces.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("trace provider shutdown: %w", err))
	}

	return errors.Join(errs...)
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

// SetReady sets the readiness state of the server.
// Called by App.Run after bootstrap completes to signal that the service
// is ready to accept traffic, and during shutdown to drain connections.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

