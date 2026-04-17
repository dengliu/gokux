// Package config provides layered configuration loading:
// YAML files (cascading) → environment variables (GOKUX_ prefix).
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

const envPrefix = "GOKUX_"

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

// Load reads configuration from YAML files (in order, later overrides earlier)
// then from environment variables with the GOKUX_ prefix.
//
// Precedence (highest wins):
//  1. Environment variables (GOKUX_SERVER_PORT, GOKUX_LOG_LEVEL, ...)
//  2. Last YAML file specified via -f
//  3. Earlier YAML files
//  4. Built-in defaults
func Load(files ...string) (*Config, error) {
	var k konf.Config

	// Load each YAML file in order; later files override earlier ones.
	for _, f := range files {
		if err := k.Load(file.New(f, file.WithUnmarshal(yaml.Unmarshal))); err != nil {
			return nil, fmt.Errorf("loading config %s: %w", f, err)
		}
	}

	// Environment variables override everything.
	// WithPrefix filters to GOKUX_* vars; WithNameSplitter strips the
	// prefix before splitting by "_" so GOKUX_SERVER_PORT maps to server.port
	// instead of gokux.server.port.
	if err := k.Load(env.New(
		env.WithPrefix(envPrefix),
		env.WithNameSplitter(func(s string) []string {
			return strings.Split(strings.TrimPrefix(s, envPrefix), "_")
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
