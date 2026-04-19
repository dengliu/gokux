// Package main demonstrates how to use the gokux library to build a
// Kubernetes-ready service with minimal boilerplate.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/dengliu/gokux"
	"github.com/dengliu/gokux/job"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	// Parse CLI flags for config file paths (-f can be repeated).
	var configFiles []string
	flag.Func("f", "config file path (can be repeated; later files override earlier)", func(s string) error {
		configFiles = append(configFiles, s)
		return nil
	})
	flag.Parse()

	// Create a stdout trace exporter for development (prints spans to stdout).
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		panic(err)
	}

	app := gokux.New(
		gokux.WithConfigFiles(configFiles...),
		gokux.WithTraceExporter(traceExporter),
	)

	// Init loads config, creates logger, and builds the server.
	// After Init, app.Logger, app.Config, and app.Server are available.
	if err := app.Init(); err != nil {
		panic(err)
	}

	// Register custom health checks.
	// Readiness: check if a dependency (e.g., database) is available.
	dbReady := &atomic.Bool{}
	dbReady.Store(true) // simulate a healthy database
	app.AddReadinessCheck("database", func(_ context.Context) error {
		if !dbReady.Load() {
			return errors.New("database connection lost")
		}

		return nil
	})

	// Create custom metrics using the OTel Meter.
	meter := app.Meter("simpleapp")
	helloCount, _ := meter.Int64Counter(
		"simpleapp.hello.count",
		metric.WithDescription("Total hello requests"),
	)

	// Create a custom tracer for application-specific spans.
	tracer := app.Tracer("simpleapp")

	// Register application routes using the Echo instance directly.
	app.Server.Echo.GET("/", func(c echo.Context) error {
		// The otelecho middleware already creates a parent span for the request.
		// Create a child span for application logic.
		ctx, span := tracer.Start(c.Request().Context(), "handle-hello",
			trace.WithAttributes(attribute.String("greeting", "hello")),
		)
		defer span.End()

		app.Logger.Info("handling request", "path", c.Path())
		helloCount.Add(ctx, 1)

		return c.JSON(http.StatusOK, map[string]string{"message": "hello"})
	})

	// Register a long-running background task with per-task shutdown.
	// This example simulates a worker that processes items every 5 seconds
	// and closes its resources when the application shuts down.
	if err := app.TaskRunner.Add(job.Task{
		Name: "background-worker",
		Run: func(ctx context.Context) error {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					app.Logger.Info("background worker tick")
				}
			}
		},
		// Shutdown receives a fresh context (not cancelled) with the
		// remaining shutdown timeout, so context-aware cleanup works.
		Shutdown: func(ctx context.Context) error {
			app.Logger.Info("background worker cleaning up")
			return nil
		},
		RestartOnFailure: true,
	}); err != nil {
		panic(err)
	}

	// Register a global shutdown callback for cross-cutting cleanup.
	app.TaskRunner.OnShutdown("flush-logs", func(ctx context.Context) error {
		app.Logger.Info("flushing logs before shutdown")
		return nil
	})

	// Run starts background tasks and the server, then blocks until SIGINT/SIGTERM.
	if err := app.Run(context.Background()); err != nil {
		panic(err)
	}
}
