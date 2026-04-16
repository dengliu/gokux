// Package main is the entrypoint for the gokux microservice.
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	slogzap "github.com/samber/slog-zap/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/dengliu/gokux/pkg/api"
	"github.com/dengliu/gokux/pkg/config"
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
	logger, err := newLogger(cfg.Log.Level)
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

	// Graceful shutdown with a 10-second timeout.
	if err := srv.Shutdown(10 * time.Second); err != nil {
		logger.Error("server shutdown error", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}

// newLogger creates a *slog.Logger backed by zap via slog-zap.
func newLogger(level string) (*slog.Logger, error) {
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

	zapLogger, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}
	defer func() { _ = zapLogger.Sync() }()

	// Map zap level to slog level.
	slogLevel := slog.LevelInfo
	switch lvl {
	case zap.DebugLevel:
		slogLevel = slog.LevelDebug
	case zap.InfoLevel:
		slogLevel = slog.LevelInfo
	case zap.WarnLevel:
		slogLevel = slog.LevelWarn
	case zap.ErrorLevel:
		slogLevel = slog.LevelError
	}

	logger := slog.New(slogzap.Option{
		Level:  slogLevel,
		Logger: zapLogger,
	}.NewZapHandler())

	return logger, nil
}
