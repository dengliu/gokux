// Package config provides 12-factor app configuration via environment variables using konf.
package config

import (
	"github.com/nil-go/konf"
	"github.com/nil-go/konf/provider/env"
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

// Load reads configuration from environment variables with the GOKUX_ prefix
// and sets it as the default konf configuration.
//
// Environment variables:
//   - GOKUX_SERVER_PORT  (default: 8080)
//   - GOKUX_LOG_LEVEL    (default: "info")
func Load() (*Config, error) {
	var k konf.Config
	if err := k.Load(env.New(env.WithPrefix("GOKUX"))); err != nil {
		return nil, err
	}
	konf.SetDefault(&k)

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
