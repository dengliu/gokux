package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, sync, err := NewLogger(level)
			require.NoError(t, err)
			assert.NotNil(t, logger)
			assert.NotNil(t, sync)
		})
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, _, err := NewLogger("invalid")
	assert.Error(t, err)
}

func TestNewLogger_EmptyLevel(t *testing.T) {
	// zap treats empty string as info level (its zero value).
	logger, sync, err := NewLogger("")
	require.NoError(t, err)
	assert.NotNil(t, logger)
	assert.NotNil(t, sync)
}

func TestNewLogger_CaseInsensitiveLevel(t *testing.T) {
	// zap's UnmarshalText is case-insensitive for level names.
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, sync, err := NewLogger(level)
			require.NoError(t, err)
			assert.NotNil(t, logger)
			assert.NotNil(t, sync)
		})
	}
}

func TestNewLogger_SyncFiltersBenignStdoutErrors(t *testing.T) {
	// Under `go test` stdout is a pipe; zap.Sync returns EINVAL/ENOTTY
	// depending on platform. The returned sync function must filter
	// those and surface nil so callers can propagate real errors.
	_, sync, err := NewLogger("info")
	require.NoError(t, err)
	require.NotNil(t, sync)
	assert.NoError(t, sync(), "sync on stdout-backed logger should filter benign errors")
}
