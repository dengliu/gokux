// Package logging provides structured logger initialization:
// zap backend with slog interface via slog-zap.
package logging

import (
	"errors"
	"log/slog"
	"os"
	"syscall"

	slogzap "github.com/samber/slog-zap/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a *slog.Logger backed by zap via slog-zap.
// The level parameter accepts standard zap level strings:
// "debug", "info", "warn", "error".
//
// The returned sync function flushes any buffered log entries and must be
// called before the process exits (typically via defer in the caller's
// shutdown path). It filters the benign errors zap's Sync returns when the
// underlying file is a terminal or pipe, so callers can propagate any
// remaining error without special-casing stdout.
func NewLogger(level string) (*slog.Logger, func() error, error) {
	lvl := zap.InfoLevel
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}

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

	return logger, syncFunc(zapLogger), nil
}

// syncFunc returns a closure that calls zap's Sync and swallows the
// well-known benign errors produced when stdout/stderr is a terminal, a
// pipe, or has already been closed. Any other error is returned as-is.
func syncFunc(zl *zap.Logger) func() error {
	return func() error {
		err := zl.Sync()
		if err == nil {
			return nil
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			switch {
			case errors.Is(pathErr.Err, syscall.ENOTTY),
				errors.Is(pathErr.Err, syscall.EINVAL),
				errors.Is(pathErr.Err, syscall.EBADF):
				return nil
			}
		}
		return err
	}
}
