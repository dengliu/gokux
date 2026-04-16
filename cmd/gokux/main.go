// Package main is the entrypoint for the gokux microservice.
package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/deng/gokux/pkg/api"
	"github.com/deng/gokux/pkg/config"
)

func main() {
	// Load 12-factor configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		// Fall back to stderr since logger may not be available.
		panic("failed to load config: " + err.Error())
	}

	// Initialize structured logger.
	logger, err := newLogger(cfg.Log.Level)
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}
	defer func() { _ = logger.Sync() }()

	// Create and start the HTTP server.
	srv := api.NewServer(cfg, logger)

	// Start the server in a goroutine.
	go func() {
		if err := srv.Start(); err != nil {
			logger.Info("server stopped", zap.Error(err))
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown with a 10-second timeout.
	if err := srv.Shutdown(10 * time.Second); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}

// newLogger creates a zap.Logger configured for production JSON output.
func newLogger(level string) (*zap.Logger, error) {
	lvl := zap.InfoLevel
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}

	zapCfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(lvl),
		Development: false,
		Encoding:    "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return zapCfg.Build()
}
