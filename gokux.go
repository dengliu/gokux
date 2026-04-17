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
//	if err := app.Run(); err != nil {
//	    panic(err)
//	}
package gokux

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dengliu/gokux/config"
	"github.com/dengliu/gokux/logging"
	"github.com/dengliu/gokux/server"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// App is a Kubernetes-ready microservice with built-in health checks,
// metrics, structured logging, and graceful shutdown.
type App struct {
	opts        options
	initialized bool
	Config      *config.Config
	Logger      *slog.Logger
	Server      *server.Server
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
	a.initialized = true

	return nil
}

// Tracer returns an OTel Tracer scoped to the given instrumentation name.
// Use it to create custom spans in application code.
// Must be called after Init.
func (a *App) Tracer(name string) trace.Tracer {
	return a.Server.TracerProvider().Tracer(name)
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
func (a *App) AddLivenessCheck(name string, check server.HealthCheck) {
	a.Server.AddLivenessCheck(name, check)
}

// AddReadinessCheck registers a named readiness check on the server.
// If any check fails (or the server is draining), /readyz returns 503.
// Must be called after Init.
func (a *App) AddReadinessCheck(name string, check server.HealthCheck) {
	a.Server.AddReadinessCheck(name, check)
}

// Run starts the server and blocks until SIGINT or SIGTERM is received.
// It then performs a graceful shutdown and returns any error.
// If Init has not been called, Run calls it automatically.
func (a *App) Run() error {
	if !a.initialized {
		if err := a.Init(); err != nil {
			return fmt.Errorf("init: %w", err)
		}
	}

	// Start the server in a goroutine.
	go func() {
		if err := a.Server.Start(); err != nil {
			a.Logger.Info("server stopped", "error", err.Error())
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	a.Logger.Info("received shutdown signal", "signal", sig.String())

	// Graceful shutdown with configurable timeout.
	if err := a.Server.Shutdown(a.Config.Server.ShutdownTimeoutDuration()); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	a.Logger.Info("server stopped gracefully")

	return nil
}
