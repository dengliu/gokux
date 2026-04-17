// Package config provides layered configuration loading:
// YAML files (cascading) → environment variables (configurable prefix).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/nil-go/konf"
	"github.com/nil-go/konf/provider/env"
	"github.com/nil-go/konf/provider/file"
	"gopkg.in/yaml.v3"
)

// DefaultEnvPrefix is the default environment variable prefix.
const DefaultEnvPrefix = "GOKUX_"

// Config holds the application configuration.
type Config struct {
	Server ServerConfig
	Log    LogConfig
}

// ServerConfig holds the HTTP server configuration.
type ServerConfig struct {
	Port                   int
	ShutdownTimeoutSeconds int `yaml:"shutdown_timeout_seconds"` // hard deadline for in-flight requests during shutdown
	DrainWaitSeconds       int `yaml:"drain_wait_seconds"`       // pause after marking not-ready, before closing listeners
}

// ShutdownTimeoutDuration returns ShutdownTimeoutSeconds as a time.Duration.
func (s ServerConfig) ShutdownTimeoutDuration() time.Duration {
	return time.Duration(s.ShutdownTimeoutSeconds) * time.Second
}

// DrainWaitDuration returns DrainWaitSeconds as a time.Duration.
func (s ServerConfig) DrainWaitDuration() time.Duration {
	return time.Duration(s.DrainWaitSeconds) * time.Second
}

// LogConfig holds the logging configuration.
type LogConfig struct {
	Level string
}

// LoadOption configures how configuration is loaded.
type LoadOption func(*loadOptions)

type loadOptions struct {
	envPrefix string
	files     []string
}

// WithEnvPrefix sets the environment variable prefix (default: "GOKUX_").
// The prefix must end with "_".
func WithEnvPrefix(prefix string) LoadOption {
	return func(o *loadOptions) {
		o.envPrefix = prefix
	}
}

// WithFiles sets the YAML config file paths to load.
func WithFiles(files ...string) LoadOption {
	return func(o *loadOptions) {
		o.files = files
	}
}

// Load reads configuration from YAML files (in order, later overrides earlier)
// then from environment variables with the configured prefix.
//
// Precedence (highest wins):
//  1. Environment variables ({PREFIX}SERVER_PORT, {PREFIX}LOG_LEVEL, ...)
//  2. Last YAML file
//  3. Earlier YAML files
//  4. Built-in defaults
func Load(opts ...LoadOption) (*Config, error) {
	o := &loadOptions{
		envPrefix: DefaultEnvPrefix,
	}
	for _, opt := range opts {
		opt(o)
	}

	var k konf.Config

	// Load each YAML file in order; later files override earlier ones.
	for _, f := range o.files {
		if err := k.Load(file.New(f, file.WithUnmarshal(yaml.Unmarshal))); err != nil {
			return nil, fmt.Errorf("loading config %s: %w", f, err)
		}
	}

	// Environment variables override everything.
	// WithPrefix filters to {PREFIX}* vars; WithNameSplitter strips the
	// prefix before splitting by "_" so {PREFIX}SERVER_PORT maps to server.port
	// instead of {prefix}.server.port.
	prefix := o.envPrefix
	if err := k.Load(env.New(
		env.WithPrefix(prefix),
		env.WithNameSplitter(func(s string) []string {
			return strings.Split(strings.TrimPrefix(s, prefix), "_")
		}),
	)); err != nil {
		return nil, err
	}
	konf.SetDefault(&k)

	// Start with built-in defaults.
	cfg := &Config{
		Server: ServerConfig{
			Port:                   8080,
			ShutdownTimeoutSeconds: 10,
			DrainWaitSeconds:       3,
		},
		Log: LogConfig{
			Level: "info",
		},
	}

	if err := konf.Unmarshal("server", &cfg.Server); err != nil {
		return nil, err
	}
	if err := konf.Unmarshal("log", &cfg.Log); err != nil {
		return nil, err
	}

	return cfg, nil
}
