// Package gokux provides a reusable framework for building Kubernetes-ready
// Go microservices with built-in health checks, Prometheus metrics,
// structured logging, and graceful shutdown.
//
// Usage:
//
//	app := gokux.New(
//	    gokux.WithConfigFiles("config.yaml"),
//	    gokux.WithRoutes(func(e *echo.Echo) {
//	        e.GET("/api/hello", helloHandler)
//	    }),
//	)
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
)

// App is a Kubernetes-ready microservice with built-in health checks,
// metrics, structured logging, and graceful shutdown.
type App struct {
	opts   options
	Config *config.Config
	Logger *slog.Logger
	Server *server.Server
}

// New creates a new App with the given Option(s).
// Call Run to start the server and block until shutdown.
func New(opts ...Option) *App {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	return &App{opts: o}
}

// Run initializes the application (config, logger, server), starts serving,
// and blocks until SIGINT or SIGTERM is received. It then performs a graceful
// shutdown and returns any error from the shutdown process.
func (a *App) Run() error {
	if err := a.init(); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// Apply user routes before starting.
	for _, register := range a.opts.routes {
		register(a.Server.Echo)
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

// init loads configuration, creates the logger, and builds the server.
func (a *App) init() error {
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

	a.Server = server.NewServer(cfg, a.Logger)

	return nil
}
