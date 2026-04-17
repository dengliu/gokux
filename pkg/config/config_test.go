package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvVarOverride(t *testing.T) {
	// Set env var with GOKUX_ prefix
	t.Setenv("GOKUX_SERVER_PORT", "9090")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port, "env var GOKUX_SERVER_PORT should override default port")
}

func TestEnvVarDrainWaitSeconds(t *testing.T) {
	// Env var names concatenate words without underscores because "_"
	// is the hierarchy delimiter (SERVER vs field name).
	t.Setenv("GOKUX_SERVER_DRAINWAITSECONDS", "7")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 7, cfg.Server.DrainWaitSeconds, "env var GOKUX_SERVER_DRAINWAITSECONDS should override default")
}

func TestEnvVarShutdownTimeoutSeconds(t *testing.T) {
	t.Setenv("GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS", "20")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 20, cfg.Server.ShutdownTimeoutSeconds, "env var GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS should override default")
}

func TestEnvVarCaseInsensitive(t *testing.T) {
	// Lowercase prefix does not match — the GOKUX_ prefix filter is case-sensitive.
	os.Setenv("gokux_server_port", "7070")
	defer os.Unsetenv("gokux_server_port")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port, "lowercase env var prefix should NOT override (case-sensitive)")
}

func TestDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 10, cfg.Server.ShutdownTimeoutSeconds)
	assert.Equal(t, 3, cfg.Server.DrainWaitSeconds)
	assert.Equal(t, "info", cfg.Log.Level)
}
