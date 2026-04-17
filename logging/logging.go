// Package logging provides structured logger initialization:
// zap backend with slog interface via slog-zap.
package logging

import (
	"log/slog"

	slogzap "github.com/samber/slog-zap/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a *slog.Logger backed by zap via slog-zap.
// The level parameter accepts standard zap level strings:
// "debug", "info", "warn", "error".
func NewLogger(level string) (*slog.Logger, error) {
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
