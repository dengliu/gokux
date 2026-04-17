// Package server provides the HTTP server, routing, and middleware for the gokux service.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/dengliu/gokux/config"
)

// Server wraps an Echo instance with application dependencies.
type Server struct {
	Echo   *echo.Echo
	Config *config.Config
	Logger *slog.Logger
	ready  *atomic.Bool
}

// NewServer creates a configured Echo server with all routes and middleware.
func NewServer(cfg *config.Config, logger *slog.Logger) *Server {
	ready := &atomic.Bool{}
	ready.Store(true)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(SlogMiddleware(logger))
	e.Use(MetricsMiddleware())

	// Health check routes
	health := newHealthHandler(ready)
	e.GET("/healthz", health.Healthz)
	e.GET("/readyz", health.Readyz)

	// Metrics route
	e.GET("/metrics", MetricsHandler())

	return &Server{
		Echo:   e,
		Config: cfg,
		Logger: logger,
		ready:  ready,
	}
}

// Start begins listening on the configured port. This call blocks.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.Config.Server.Port)
	s.Logger.Info("starting server", "addr", addr)

	return s.Echo.Start(addr)
}

// Shutdown performs a graceful shutdown: marks the service as not-ready,
// waits for in-flight requests to drain, then stops the server.
func (s *Server) Shutdown(timeout time.Duration) error {
	s.Logger.Info("shutting down server")

	// Mark as not ready so readiness probe fails and
	// Kubernetes stops sending new traffic.
	s.ready.Store(false)

	// Allow time for load balancers to detect the readiness change.
	drainWait := s.Config.Server.DrainWaitDuration()
	s.Logger.Info("waiting for in-flight requests to drain", "drain", drainWait)
	time.Sleep(drainWait)

	// Create a context with a timeout for the shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return s.Echo.Shutdown(ctx)
}

// SlogMiddleware returns an Echo middleware that logs each request using slog.
func SlogMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
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
