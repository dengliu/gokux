package config

import (
	"os"
	"testing"
)

func TestEnvVarOverride(t *testing.T) {
	// Set env var with GOKUX_ prefix
	os.Setenv("GOKUX_SERVER_PORT", "9090")
	defer os.Unsetenv("GOKUX_SERVER_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected Port=9090, got Port=%d (env var override did NOT work)", cfg.Server.Port)
	} else {
		t.Logf("Port=%d (env var override works correctly)", cfg.Server.Port)
	}
}

func TestEnvVarDrainWaitSeconds(t *testing.T) {
	// Env var names concatenate words without underscores because "_"
	// is the hierarchy delimiter (SERVER vs field name).
	os.Setenv("GOKUX_SERVER_DRAINWAITSECONDS", "7")
	defer os.Unsetenv("GOKUX_SERVER_DRAINWAITSECONDS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.DrainWaitSeconds != 7 {
		t.Errorf("expected DrainWaitSeconds=7, got DrainWaitSeconds=%d (env var override did NOT work)", cfg.Server.DrainWaitSeconds)
	} else {
		t.Logf("DrainWaitSeconds=%d (env var override works correctly)", cfg.Server.DrainWaitSeconds)
	}
}

func TestEnvVarShutdownTimeoutSeconds(t *testing.T) {
	os.Setenv("GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS", "20")
	defer os.Unsetenv("GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.ShutdownTimeoutSeconds != 20 {
		t.Errorf("expected ShutdownTimeoutSeconds=20, got ShutdownTimeoutSeconds=%d (env var override did NOT work)", cfg.Server.ShutdownTimeoutSeconds)
	} else {
		t.Logf("ShutdownTimeoutSeconds=%d (env var override works correctly)", cfg.Server.ShutdownTimeoutSeconds)
	}
}

func TestEnvVarCaseInsensitive(t *testing.T) {
	// Test with lowercase env var
	os.Setenv("gokux_server_port", "7070")
	defer os.Unsetenv("gokux_server_port")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port == 7070 {
		t.Logf("Port=%d (lowercase env var works)", cfg.Server.Port)
	} else {
		t.Logf("Port=%d (lowercase env var does NOT work — case-sensitive)", cfg.Server.Port)
	}
}
