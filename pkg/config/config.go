// Package config provides layered configuration loading:
// YAML files (cascading) → environment variables (GOKUX_ prefix).
package config

import (
	"fmt"

	"github.com/nil-go/konf"
	"github.com/nil-go/konf/provider/env"
	"github.com/nil-go/konf/provider/file"
)

// Config holds the application configuration.
type Config struct {
	Server ServerConfig
	Log    LogConfig
}

// ServerConfig holds the HTTP server configuration.
type ServerConfig struct {
	Port int
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
		if err := k.Load(file.New(f)); err != nil {
			return nil, fmt.Errorf("loading config %s: %w", f, err)
		}
	}

	// Environment variables override everything.
	if err := k.Load(env.New(env.WithPrefix("GOKUX"))); err != nil {
		return nil, err
	}
	konf.SetDefault(&k)

	// Start with built-in defaults.
	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
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
