package gokux

import (
	"log/slog"
)

// Option configures an App.
type Option func(*options)

type options struct {
	configFiles []string
	envPrefix   string
	logger      *slog.Logger
}

// WithConfigFiles sets the YAML config file paths to load.
// Later files override earlier ones, similar to
// helm install -f values.yaml -f values-dev.yaml.
func WithConfigFiles(files ...string) Option {
	return func(o *options) {
		o.configFiles = files
	}
}

// WithEnvPrefix sets the environment variable prefix (default: "GOKUX_").
// For example, WithEnvPrefix("MYAPP_") makes the app read MYAPP_SERVER_PORT
// instead of GOKUX_SERVER_PORT.
func WithEnvPrefix(prefix string) Option {
	return func(o *options) {
		o.envPrefix = prefix
	}
}

// WithLogger provides a pre-configured logger instead of creating one
// from the log.level config value.
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}
