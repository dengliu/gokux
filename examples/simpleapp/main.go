// Package main demonstrates how to use the gokux framework to build a
// Kubernetes-ready service with minimal boilerplate.
package main

import (
	"errors"
	"flag"
	"net/http"
	"sync/atomic"

	"github.com/dengliu/gokux"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	// Parse CLI flags for config file paths (-f can be repeated).
	var configFiles []string
	flag.Func("f", "config file path (can be repeated; later files override earlier)", func(s string) error {
		configFiles = append(configFiles, s)
		return nil
	})
	flag.Parse()

	app := gokux.New(
		gokux.WithConfigFiles(configFiles...),
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
	app.AddReadinessCheck("database", func() error {
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

	// Register application routes using the Echo instance directly.
	app.Server.Echo.GET("/", func(c echo.Context) error {
		app.Logger.Info("handling request", "path", c.Path())
		helloCount.Add(c.Request().Context(), 1)

		return c.JSON(http.StatusOK, map[string]string{"message": "hello"})
	})

	// Run starts the server and blocks until SIGINT/SIGTERM.
	if err := app.Run(); err != nil {
		panic(err)
	}
}
