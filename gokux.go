// Package gokux provides a reusable framework for building Kubernetes-ready
// Go microservices with built-in health checks, Prometheus metrics,
// structured logging, and graceful shutdown.
//
// Usage:
//
//	app := gokux.New(gokux.WithConfigFiles("config.yaml"))
//	if err := app.Init(); err != nil {
//	    panic(err)
//	}
//
//	app.Server.Echo.GET("/api/hello", helloHandler)
//
//	if err := app.Run(context.Background()); err != nil {
//	    panic(err)
//	}
package gokux

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dengliu/gokux/config"
	"github.com/dengliu/gokux/job"
	"github.com/dengliu/gokux/logging"
	"github.com/dengliu/gokux/server"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// HealthCheck is a function that reports the health of a dependency.
// Return nil if healthy, or an error describing the problem.
type HealthCheck = server.HealthCheck

// App is a Kubernetes-ready microservice with built-in health checks,
// metrics, structured logging, graceful shutdown, and background task
// management.
type App struct {
	opts        options
	initialized bool
	Config      *config.Config
	Logger      *slog.Logger
	Server      *server.Server

	// TaskRunner manages long-running background goroutines and shutdown
	// callbacks. Use Add to register tasks before Run, and OnShutdown
	// to register cleanup hooks.
	TaskRunner *job.TaskRunner
}

// New creates a new App with the given Option(s).
// Call Init to initialize, then register routes, then call Run.
func New(opts ...Option) *App {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	return &App{opts: o}
}

// Init loads configuration, creates the logger, and builds the server.
// After Init returns, Config, Logger, and Server are available for
// registering routes, middleware, and other setup before calling Run.
func (a *App) Init() error {
	if a.initialized {
		return nil
	}

	// Build config load options.
	var cfgOpts []config.LoadOption
	if len(a.opts.configFiles) > 0 {
		cfgOpts = append(cfgOpts, config.WithFiles(a.opts.configFiles...))
	}
	if a.opts.envPrefix != "" {
		cfgOpts = append(cfgOpts, config.WithEnvPrefix(a.opts.envPrefix))
	}

	cfg, err := config.Load(cfgOpts...)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	a.Config = cfg

	// Use provided logger or create one from config.
	if a.opts.logger != nil {
		a.Logger = a.opts.logger
	} else {
		logger, err := logging.NewLogger(cfg.Log.Level)
		if err != nil {
			return fmt.Errorf("create logger: %w", err)
		}
		a.Logger = logger
	}

	srv, err := server.NewServer(cfg, a.Logger, a.opts.traceExporter)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	a.Server = srv
	a.TaskRunner = job.NewTaskRunner(a.Logger)
	a.initialized = true

	return nil
}

// Tracer returns an OTel Tracer scoped to the given instrumentation name.
// Use it to create custom spans in application code.
// Safe to call even when tracing is not configured (returns noop tracer).
// Must be called after Init.
func (a *App) Tracer(name string) trace.Tracer {
	return a.Server.Tracer(name)
}

// Meter returns an OTel Meter scoped to the given instrumentation name.
// Use it to create custom counters, histograms, and gauges that
// automatically appear on the /metrics endpoint.
// Must be called after Init.
func (a *App) Meter(name string) metric.Meter {
	return a.Server.MeterProvider().Meter(name)
}

// AddLivenessCheck registers a named liveness check on the server.
// If any check fails, /healthz returns 503.
// Must be called after Init.
func (a *App) AddLivenessCheck(name string, check HealthCheck) {
	a.Server.AddLivenessCheck(name, check)
}

// AddReadinessCheck registers a named readiness check on the server.
// If any check fails (or the server is draining), /readyz returns 503.
// Must be called after Init.
func (a *App) AddReadinessCheck(name string, check HealthCheck) {
	a.Server.AddReadinessCheck(name, check)
}

// Run starts background tasks and the HTTP server, then blocks until the
// context is canceled or SIGINT/SIGTERM is received. It performs a graceful
// shutdown of tasks first (so they can still use the server if needed),
// then the server itself.
// If Init has not been called, Run calls it automatically.
func (a *App) Run(ctx context.Context) error {
	if !a.initialized {
		if err := a.Init(); err != nil {
			return fmt.Errorf("init: %w", err)
		}
	}

	// Start background tasks.
	a.TaskRunner.Start(ctx)

	// Start the server in a goroutine.
	go func() {
		if err := a.Server.Start(); err != nil {
			a.Logger.Info("server stopped", "error", err.Error())
		}
	}()

	// Wait for context cancellation or interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		a.Logger.Info("received shutdown signal", "signal", sig.String())
	case <-ctx.Done():
		a.Logger.Info("context canceled")
	}

	shutdownTimeout := a.Config.Server.ShutdownTimeoutDuration()

	// Shutdown tasks first — they may depend on the server being up.
	if err := a.TaskRunner.Shutdown(shutdownTimeout); err != nil {
		a.Logger.Error("task runner shutdown error", "error", err)
	}

	// Graceful shutdown of the HTTP server.
	if err := a.Server.Shutdown(shutdownTimeout); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	a.Logger.Info("server stopped gracefully")

	return nil
}
