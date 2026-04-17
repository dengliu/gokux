// Package main demonstrates how to use the gokux framework to build a
// Kubernetes-ready service with minimal boilerplate.
package main

import (
	"flag"
	"net/http"

	"github.com/dengliu/gokux"
	"github.com/labstack/echo/v4"
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

	// Register application routes using the Echo instance directly.
	app.Server.Echo.GET("/", func(c echo.Context) error {
		app.Logger.Info("handling request", "path", c.Path())

		return c.JSON(http.StatusOK, map[string]string{"message": "hello"})
	})

	// Run starts the server and blocks until SIGINT/SIGTERM.
	if err := app.Run(); err != nil {
		panic(err)
	}
}
