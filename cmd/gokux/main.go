// Package main is the entrypoint for the gokux microservice.
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/dengliu/gokux/pkg/api"
	"github.com/dengliu/gokux/pkg/config"
	"github.com/dengliu/gokux/pkg/logging"
)

func main() {
	// Parse CLI flags for config file paths (-f can be repeated).
	var configFiles []string
	flag.Func("f", "config file path (can be repeated; later files override earlier)", func(s string) error {
		configFiles = append(configFiles, s)
		return nil
	})
	flag.Parse()

	// Load layered configuration: YAML files → env vars.
	cfg, err := config.Load(configFiles...)
	if err != nil {
		// Fall back to stderr since logger may not be available.
		panic("failed to load config: " + err.Error())
	}

	// Initialize structured logger: zap backend with slog interface.
	logger, err := logging.NewLogger(cfg.Log.Level)
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}

	// Create and start the HTTP server.
	srv := api.NewServer(cfg, logger)

	// Start the server in a goroutine.
	go func() {
		if err := srv.Start(); err != nil {
			logger.Info("server stopped", "error", err.Error())
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("received shutdown signal", "signal", sig.String())

	// Graceful shutdown with configurable timeout.
	if err := srv.Shutdown(cfg.Server.ShutdownTimeoutDuration()); err != nil {
		logger.Error("server shutdown error", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}
